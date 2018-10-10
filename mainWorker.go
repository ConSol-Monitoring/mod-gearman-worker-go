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

var activeWorkers int

var workerSlice []*worker

var activeChan chan int

var min1, min5, min15 float32

func startMinWorkers() {
	//channel for communication
	activeChan = make(chan int)

	for i := 0; i < config.minWorker; i++ {
		worker := newWorker(activeChan)
		workerSlice = append(workerSlice, worker)
	}

	//tick signal for starting new workers if needed
	tick := time.Tick(1 * time.Second)

	//get first load avg
	getLoadAvg()

	for {
		select {
		case x := <-activeChan:
			activeWorkers += x
		case <-tick:
			startNewWorkers()
		}

	}

}

// reads the avg loads from /procs/loadavg
func getLoadAvg() {
	file, err := os.Open("/proc/loadavg")
	if err == nil {
		scanner := bufio.NewScanner(file)
		//read first line:
		scanner.Scan()
		firstline := scanner.Text()
		values := strings.Split(firstline, " ")

		min1 = getFloat(values[0])
		min5 = getFloat(values[1])
		min15 = getFloat(values[2])

	}
}

//checks if all the loadlimits get checked, when values are set
func checkLoads() bool {
	if config.loadLimit1 != 0 && min1 != 0 {
		if config.loadLimit1 < min1 {
			return false
		}
	}

	if config.loadLimit5 != 0 && min5 != 0 {
		if config.loadLimit5 < min5 {
			return false
		}
	}

	if config.loadLimit15 != 0 && min15 != 0 {
		if config.loadLimit15 < min15 {
			return false
		}
	}

	return true
}

//starts new workers if all workers are busy and the loads are not to high
func startNewWorkers() {
	//get new load avg
	getLoadAvg()
	if (activeWorkers == len(workerSlice) && len(workerSlice) < config.maxWorker &&
		checkLoads()) ||
		len(workerSlice) < config.minWorker {
		//start new workers at spawn speed from the configuration file
		for i := 0; i < config.spawnRate; i++ {
			worker := newWorker(activeChan)
			workerSlice = append(workerSlice, worker)
		}
	}
}

/*
* removes the connection to the server from the worker
* then removes the worker from the slice
 */
func removeWorker(worker *worker) {
	//first remove the worker from the list, only if there are enough workers left
	if len(workerSlice) > config.minWorker {
		worker.closeWorker()
		removeFromSlice(worker)
	} else {
		//reset max jobs counter and idle time counter
		worker.maxJobs = config.maxJobs
		worker.idleSince = time.Now()
		worker.startIdleTimer()
	}
}

/*
* Helper to remove the worker from the
* slice of workers
 */
func removeFromSlice(worker *worker) {
	for i, v := range workerSlice {
		if v == worker {
			//copy everything after found one to the left
			//nill the last value so no memory leaks appear
			copy(workerSlice[i:], workerSlice[i+1:])
			workerSlice[len(workerSlice)-1] = nil
			workerSlice = workerSlice[:len(workerSlice)-1]
			return
		}
	}
}
