package main

import (
	"bufio"
	"os"
	"strings"
	"sync"
	time "time"
)

/*
* starts the min workers
* manages the worker list
* spawns new workers if needed
* kills worker being to old
 */

type mainWorker struct {
	activeWorkers int
	workerMap     map[string]*worker
	workerMapLock *sync.RWMutex
	statusWorker  *worker
	activeChan    chan int
	min1          float32
	min5          float32
	min15         float32
	config        *configurationStruct
	key           []byte
	tasks         int
	idleSince     time.Time
}

func newMainWorker(configuration *configurationStruct, key []byte) *mainWorker {
	return &mainWorker{
		activeWorkers: 0,
		activeChan:    make(chan int, 100),
		key:           key,
		config:        configuration,
		workerMap:     make(map[string]*worker),
		workerMapLock: new(sync.RWMutex),
		idleSince:     time.Now(),
	}
}

func (w *mainWorker) managerWorkerLoop(shutdownChannel chan bool) {
	ticker := time.NewTicker(1 * time.Second)
	for {
		select {
		case x := <-w.activeChan:
			w.activeWorkers += x
		case <-ticker.C:
			w.manageWorkers()
		case <-shutdownChannel:
			logger.Debugf("managerWorkerLoop ending...")
			ticker.Stop()
			for _, worker := range w.workerMap {
				logger.Debugf("worker removed...")
				worker.Shutdown()
			}
			if w.statusWorker != nil {
				logger.Debugf("statusworker removed...")
				w.statusWorker.Shutdown()
				w.statusWorker = nil
			}
			return
		}
	}
}

func (w *mainWorker) manageWorkers() {
	// start status worker
	if w.statusWorker == nil {
		w.statusWorker = newStatusWorker(w.config, w)
	}

	totalWorker := len(w.workerMap)
	logger.Debugf("manageWorkers: total: %d, active: %d (min: %d, max: %d)", totalWorker, w.activeWorkers, w.config.minWorker, w.config.maxWorker)
	workerCount.Set(float64(totalWorker))
	workingWorkerCount.Set(float64(w.activeWorkers))
	idleWorkerCount.Set(float64(totalWorker - w.activeWorkers))

	//as long as there are to few workers start them without a limit
	for i := w.config.minWorker - len(w.workerMap); i > 0; i-- {
		logger.Debugf("manageWorkers: starting minworker: %d, %d", w.config.minWorker-len(w.workerMap), i)
		worker := newWorker(w.activeChan, w.config, w.key, w)
		w.registerWorker(worker)
		w.idleSince = time.Now()
	}

	//check if we have too many workers (less than 90% active and above minWorker)
	if (w.activeWorkers/len(w.workerMap)*100) < 90 && len(w.workerMap) > w.config.minWorker && (time.Now().Unix()-w.idleSince.Unix() > w.config.idleTimeout) {
		//reduce workers at spawnrate
		for i := 0; i < w.config.spawnRate; i++ {
			if len(w.workerMap) <= w.config.minWorker {
				break
			}
			// stop first idle worker
			logger.Debugf("manageWorkers: stopping one...")
			for _, worker := range w.workerMap {
				if worker.idle {
					worker.Shutdown()
					break
				}
			}
		}
	}

	//check if we need more workers
	if w.activeWorkers == len(w.workerMap) && len(w.workerMap) < w.config.maxWorker {
		if !w.checkLoads() {
			return
		}
		//start new workers at spawn speed from the configuration file
		for i := 0; i < w.config.spawnRate; i++ {
			if len(w.workerMap) >= w.config.maxWorker {
				break
			}
			logger.Debugf("manageWorkers: starting one...")
			worker := newWorker(w.activeChan, w.config, w.key, w)
			w.registerWorker(worker)
			w.idleSince = time.Now()
		}
	}
}

// reads the avg loads from /procs/loadavg
func (w *mainWorker) getLoadAvg() {
	file, err := os.Open("/proc/loadavg")
	if err != nil {
		return
	}
	scanner := bufio.NewScanner(file)
	//read first line:
	scanner.Scan()
	firstline := scanner.Text()
	values := strings.Split(firstline, " ")

	w.min1 = getFloat(values[0])
	w.min5 = getFloat(values[1])
	w.min15 = getFloat(values[2])
}

//checks if all the loadlimits get checked, when values are set
func (w *mainWorker) checkLoads() bool {
	if w.config.loadLimit1 <= 0 && w.config.loadLimit5 <= 0 && w.config.loadLimit15 <= 0 {
		return true
	}

	w.getLoadAvg()
	if w.config.loadLimit1 > 0 && w.min1 > 0 && w.config.loadLimit1 < w.min1 {
		logger.Debugf("not starting any more worker, load1 is too high: %f > %f", w.min1, w.config.loadLimit1)
		return false
	}

	if w.config.loadLimit5 > 0 && w.min5 > 0 && w.config.loadLimit5 < w.min5 {
		logger.Debugf("not starting any more worker, load5 is too high: %f > %f", w.min5, w.config.loadLimit5)
		return false
	}

	if w.config.loadLimit15 > 0 && w.min15 > 0 && w.config.loadLimit15 < w.min15 {
		logger.Debugf("not starting any more worker, load15 is too high: %f > %f", w.min15, w.config.loadLimit15)
		return false
	}

	return true
}

func (w *mainWorker) unregisterWorker(worker *worker) {
	w.workerMapLock.Lock()
	delete(w.workerMap, worker.id)
	w.workerMapLock.Unlock()
}

func (w *mainWorker) registerWorker(worker *worker) {
	if worker == nil {
		return
	}
	w.workerMapLock.Lock()
	w.workerMap[worker.id] = worker
	w.workerMapLock.Unlock()
}
