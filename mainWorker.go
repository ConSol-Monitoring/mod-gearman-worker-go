package modgearman

import (
	"bufio"
	"net"
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
	min1          float64
	min5          float64
	min15         float64
	config        *configurationStruct
	key           []byte
	tasks         int
	idleSince     time.Time
	serverStatus  map[string]string
	running       bool
}

func newMainWorker(configuration *configurationStruct, key []byte, workerMap *map[string]*worker) *mainWorker {
	w := &mainWorker{
		activeWorkers: 0,
		key:           key,
		config:        configuration,
		workerMap:     *workerMap,
		workerMapLock: new(sync.RWMutex),
		idleSince:     time.Now(),
		serverStatus:  make(map[string]string),
	}
	w.RetryFailedConnections()
	return w
}

func (w *mainWorker) managerWorkerLoop(shutdownChannel chan bool, initialStart int) {
	w.running = true
	defer func() { w.running = false }()

	// check connections
	go func() {
		defer logPanicExit()
		for w.running {
			if w.RetryFailedConnections() {
				w.StopAllWorker()
			}
			time.Sleep(3 * time.Second)
		}
	}()

	w.manageWorkers(initialStart)
	ticker := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-ticker.C:
			w.manageWorkers(0)
		case <-shutdownChannel:
			logger.Debug("managerWorkerLoop ending...")
			ticker.Stop()
			w.StopAllWorker()
			return
		}
	}
}

func (w *mainWorker) manageWorkers(initialStart int) {
	// if there are no servers, we cannot do anything
	if len(w.ActiveServerList()) == 0 {
		logger.Tracef("manageWorkers: no active servers available, retrying...")
		return
	}

	// start status worker
	if w.statusWorker == nil {
		w.statusWorker = newStatusWorker(w.config, w)
	}

	activeWorkers := 0
	totalWorker := len(w.workerMap)
	for _, w := range w.workerMap {
		if !w.idle {
			activeWorkers++
		}
	}
	w.activeWorkers = activeWorkers
	logger.Tracef("manageWorkers: total: %d, active: %d (min: %d, max: %d)", totalWorker, activeWorkers, w.config.minWorker, w.config.maxWorker)
	workerCount.Set(float64(totalWorker))
	workingWorkerCount.Set(float64(activeWorkers))
	idleWorkerCount.Set(float64(totalWorker - activeWorkers))

	//as long as there are to few workers start them without a limit
	minWorker := w.config.minWorker
	if initialStart > 0 {
		minWorker = initialStart
	}
	logger.Tracef("manageWorkers: total: %d, active: %d, minWorker: %d", totalWorker, activeWorkers, minWorker)
	for i := minWorker - len(w.workerMap); i > 0; i-- {
		logger.Tracef("manageWorkers: starting minworker: %d, %d", minWorker-len(w.workerMap), i)
		worker := newWorker("check", w.config, w)
		w.registerWorker(worker)
		w.idleSince = time.Now()
	}

	//check if we have too many workers
	w.adjustWorkerBottomLevel()

	//check if we need more workers
	w.adjustWorkerTopLevel()
}

//check if we need more workers and start new ones
func (w *mainWorker) adjustWorkerTopLevel() {
	// only if all are busy
	if w.activeWorkers < len(w.workerMap) {
		return
	}
	// do not exceed maxWorker level
	if len(w.workerMap) >= w.config.maxWorker {
		return
	}
	// check load levels
	if !w.checkLoads() {
		return
	}

	//start new workers at spawn speed
	for i := 0; i < w.config.spawnRate; i++ {
		if len(w.workerMap) >= w.config.maxWorker {
			break
		}
		logger.Debugf("manageWorkers: starting one...")
		worker := newWorker("check", w.config, w)
		w.registerWorker(worker)
		w.idleSince = time.Now()
	}
}

//check if we have too many workers (less than 90% active and above minWorker)
func (w *mainWorker) adjustWorkerBottomLevel() {
	if len(w.workerMap) <= 0 {
		return
	}
	// below minmum level
	if len(w.workerMap) <= w.config.minWorker {
		return
	}
	// above 90% utilization
	if (w.activeWorkers / len(w.workerMap) * 100) >= 90 {
		return
	}
	// not idling long enough
	if time.Now().Unix()-w.idleSince.Unix() <= int64(w.config.idleTimeout) {
		return
	}

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
	values := strings.Fields(firstline)

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
	switch worker.what {
	case "check":
		w.workerMapLock.Lock()
		delete(w.workerMap, worker.id)
		w.workerMapLock.Unlock()
	case "status":
		w.statusWorker = nil
	}
}

func (w *mainWorker) registerWorker(worker *worker) {
	if worker == nil {
		return
	}
	switch worker.what {
	case "check":
		w.workerMapLock.Lock()
		w.workerMap[worker.id] = worker
		w.workerMapLock.Unlock()
	case "status":
		if w.statusWorker != nil {
			logger.Errorf("duplicate status worker started")
		}
		w.statusWorker = worker
	}
}

// RetryFailedConnections updates status of failed servers
// returns true if server list has changed
func (w *mainWorker) RetryFailedConnections() bool {
	changed := false
	for _, address := range w.config.server {
		w.workerMapLock.RLock()
		status, ok := w.serverStatus[address]
		w.workerMapLock.RUnlock()
		previous := true
		if !ok || status != "" {
			previous = false
		}
		_, err := net.DialTimeout("tcp", address, 30*time.Second)
		w.workerMapLock.Lock()
		if err != nil {
			w.serverStatus[address] = err.Error()
			if previous {
				changed = true
			}
		} else {
			w.serverStatus[address] = ""
			if !previous {
				changed = true
			}
		}
		w.workerMapLock.Unlock()
	}
	return changed
}

// ActiveServerList returns list of active servers
func (w *mainWorker) ActiveServerList() (servers []string) {
	w.workerMapLock.RLock()
	defer w.workerMapLock.RUnlock()

	servers = make([]string, 0)
	for _, address := range w.config.server {
		if w.serverStatus[address] == "" {
			servers = append(servers, address)
		}
	}
	return
}

// GetServerStatus returns server status for given address
func (w *mainWorker) GetServerStatus(addr string) (err string) {
	w.workerMapLock.RLock()
	defer w.workerMapLock.RUnlock()

	err = w.serverStatus[addr]
	return err
}

// SetServerStatus sets server status for given address
func (w *mainWorker) SetServerStatus(addr, err string) {
	w.workerMapLock.Lock()
	defer w.workerMapLock.Unlock()
	w.serverStatus[addr] = err
}

// StopAllWorker stops all check worker and the status worker
func (w *mainWorker) StopAllWorker() {
	w.workerMapLock.RLock()
	workerMap := w.workerMap
	workerNum := len(workerMap)
	w.workerMapLock.RUnlock()
	exited := make(chan int, 1)
	for _, wo := range workerMap {
		logger.Debugf("worker removed...")
		go func(wo *worker, ch chan int) {
			defer logPanicExit()
			// this might take a while, because it waits for the current job to complete
			wo.Shutdown()
			ch <- 1
		}(wo, exited)
	}

	// wait 10 seconds to end all worker
	timeout := time.NewTimer(10 * time.Second)
	alreadyExited := 0
	for {
		select {
		case <-timeout.C:
			logger.Debugf("timeout while waiting for all workers to stop, already stopped: %d/%d", alreadyExited, workerNum)
			w.StopStatusWorker()
			return
		case <-exited:
			logger.Tracef("worker exiting %d/%d", alreadyExited, workerNum)
			alreadyExited++
			if alreadyExited >= workerNum {
				w.StopStatusWorker()
				return
			}
			timeout.Stop()
		}
	}
}

// StopStatusWorker stops all check worker and the status worker
func (w *mainWorker) StopStatusWorker() {
	if w.statusWorker == nil {
		return
	}
	logger.Debugf("statusworker removed...")
	w.statusWorker.Shutdown()
	w.statusWorker = nil
}
