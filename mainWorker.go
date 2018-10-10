package main

import (
	"bufio"
	"os"
	"strings"
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
	workerSlice   []*worker
	activeChan    chan int
	min1          float32
	min5          float32
	min15         float32
	config        *configurationStruct
	key           []byte
}

func newMainWorker(configuration *configurationStruct, key []byte) *mainWorker {
	return &mainWorker{
		activeWorkers: 0,
		activeChan:    make(chan int),
		key:           key,
		config:        configuration,
	}
}

func (w *mainWorker) startWorker() {
	tick := time.Tick(1 * time.Second)

	//get first load avg
	w.getLoadAvg()

	for {
		select {
		case x := <-w.activeChan:
			w.activeWorkers += x
		case <-tick:
			w.manageWorkers()
		}

	}
}

func (w *mainWorker) manageWorkers() {

	//as long as there are to few workers start them without a limit

	for i := w.config.minWorker - len(w.workerSlice); i > 0; i-- {
		worker := newWorker(w.activeChan, w.config, w.key, w)
		if worker != nil {
			w.workerSlice = append(w.workerSlice, worker)
		}
	}

	//check if we need more workers
	w.getLoadAvg()
	if w.activeWorkers == len(w.workerSlice) && len(w.workerSlice) < w.config.maxWorker &&
		w.checkLoads() {
		//start new workers at spawn speed from the configuration file
		for i := 0; i < w.config.spawnRate; i++ {
			worker := newWorker(w.activeChan, w.config, w.key, w)
			if worker != nil {
				w.workerSlice = append(w.workerSlice, worker)
			}
		}
	}

}

// reads the avg loads from /procs/loadavg
func (w *mainWorker) getLoadAvg() {
	file, err := os.Open("/proc/loadavg")
	if err == nil {
		scanner := bufio.NewScanner(file)
		//read first line:
		scanner.Scan()
		firstline := scanner.Text()
		values := strings.Split(firstline, " ")

		w.min1 = getFloat(values[0])
		w.min5 = getFloat(values[1])
		w.min15 = getFloat(values[2])

	}
}

//checks if all the loadlimits get checked, when values are set
func (w *mainWorker) checkLoads() bool {
	if w.config.loadLimit1 != 0 && w.min1 != 0 {
		if w.config.loadLimit1 < w.min1 {
			return false
		}
	}

	if w.config.loadLimit5 != 0 && w.min5 != 0 {
		if w.config.loadLimit5 < w.min5 {
			return false
		}
	}

	if w.config.loadLimit15 != 0 && w.min15 != 0 {
		if w.config.loadLimit15 < w.min15 {
			return false
		}
	}

	return true
}

/*
* removes the connection to the server from the worker
* then removes the worker from the slice
 */
func (w *mainWorker) removeWorker(worker *worker) {
	//first remove the worker from the list, only if there are enough workers left
	worker.closeWorker()
	w.removeFromSlice(worker)
}

/*
* Helper to remove the worker from the
* slice of workers
 */
func (w *mainWorker) removeFromSlice(worker *worker) {
	for i, v := range w.workerSlice {
		if v == worker {
			//copy everything after found one to the left
			//nill the last value so no memory leaks appear
			copy(w.workerSlice[i:], w.workerSlice[i+1:])
			w.workerSlice[len(w.workerSlice)-1] = nil
			w.workerSlice = w.workerSlice[:len(w.workerSlice)-1]
			return
		}
	}
}
