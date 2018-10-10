package main

import (
	"bufio"
	"os"
	"strings"
	"time"
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

// var activeWorkers int

// var workerSlice []*worker

// var activeChan chan int

// var min1, min5, min15 float32

func newMainWorker(configuration *configurationStruct, key []byte) *mainWorker {
	return &mainWorker{
		activeWorkers: 0,
		activeChan:    make(chan int),
		key:           key,
		config:        configuration,
	}
}

func (w *mainWorker) startMinWorkers() {
	//channel for communication
	// w.activeChan = make(chan int)
	// key := getKey()

	for i := 0; i < w.config.minWorker; i++ {
		worker := newWorker(w.activeChan, w.config, w.key, w)
		if worker != nil {
			w.workerSlice = append(w.workerSlice, worker)
		}
	}

	//tick signal for starting new workers if needed
	tick := time.Tick(1 * time.Second)

	//get first load avg
	w.getLoadAvg()

	for {
		select {
		case x := <-w.activeChan:
			w.activeWorkers += x
		case <-tick:
			w.startNewWorkers()
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

//starts new workers if all workers are busy and the loads are not to high
func (w *mainWorker) startNewWorkers() {
	//get new load avg
	w.getLoadAvg()
	if (w.activeWorkers == len(w.workerSlice) && len(w.workerSlice) < w.config.maxWorker &&
		w.checkLoads()) ||
		len(w.workerSlice) < w.config.minWorker {
		//start new workers at spawn speed from the configuration file
		for i := 0; i < w.config.spawnRate; i++ {
			worker := newWorker(w.activeChan, w.config, w.key, w)
			if worker != nil {
				w.workerSlice = append(w.workerSlice, worker)
			}
		}
	}
}

/*
* removes the connection to the server from the worker
* then removes the worker from the slice
 */
func (w *mainWorker) removeWorker(worker *worker) {
	//first remove the worker from the list, only if there are enough workers left
	if len(w.workerSlice) > w.config.minWorker {
		worker.closeWorker()
		w.removeFromSlice(worker)
	} else {
		//reset max jobs counter and idle time counter
		worker.maxJobs = w.config.maxJobs
		worker.idleSince = time.Now()
		worker.startIdleTimer()
	}
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
