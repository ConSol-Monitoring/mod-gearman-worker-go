package main

import (
	"fmt"
	"runtime/debug"

	libworker "github.com/appscode/g2/worker"
)

type worker struct {
	id         string
	worker     *libworker.Worker
	idle       bool
	start      chan int
	config     *configurationStruct
	key        []byte
	mainWorker *mainWorker
	tasks      int
}

//creates a new worker and returns a pointer to it
// counterChanel will receive +1 if a job is received and started
// and -1 if a job is completed
func newWorker(counterChanel chan int, configuration *configurationStruct, key []byte, mainWorker *mainWorker) *worker {
	logger.Tracef("starting new worker")
	worker := &worker{
		idle:       true,
		start:      counterChanel,
		config:     configuration,
		key:        key,
		mainWorker: mainWorker,
	}
	worker.id = fmt.Sprintf("%p", worker)

	w := libworker.New(libworker.OneByOne)
	worker.worker = w

	w.ErrorHandler = func(e error) {
		logger.Errorf(e.Error())
		logger.Errorf("%s", debug.Stack())
		worker.Shutdown()
	}

	//listen to this servers
	for _, address := range worker.config.server {
		err := w.AddServer("tcp4", address)
		if err != nil {
			logger.Error(err)
		}
	}

	// specifies what events the worker listens
	if worker.config.eventhandler {
		w.AddFunc("eventhandler", worker.doWork, libworker.Unlimited)
	}
	if worker.config.hosts {
		w.AddFunc("host", worker.doWork, libworker.Unlimited)
	}
	if worker.config.services {
		w.AddFunc("service", worker.doWork, libworker.Unlimited)
	}
	if worker.config.notifications {
		w.AddFunc("notification", worker.doWork, libworker.Unlimited)
	}

	//register for the hostgroups
	if len(worker.config.hostgroups) > 0 {
		for _, element := range worker.config.hostgroups {
			w.AddFunc("hostgroup_"+element, worker.doWork, libworker.Unlimited)
		}
	}

	//register for servicegroups
	if len(worker.config.servicegroups) > 0 {
		for _, element := range worker.config.servicegroups {
			w.AddFunc("servicegroup_"+element, worker.doWork, libworker.Unlimited)
		}
	}

	//check if worker is ready
	if err := w.Ready(); err != nil {
		logger.Debugf("worker not ready closing again: %s", err.Error())
		worker.Shutdown()
		return nil
	}
	//start the worker
	go func() {
		defer logPanicExit()
		w.Work()
	}()

	return worker
}

func (worker *worker) doWork(job libworker.Job) ([]byte, error) {
	logger.Debugf("worker got a job: %s", job.Handle())

	//set worker to idle
	worker.idle = false
	worker.start <- 1

	defer func() {
		worker.start <- -1
		worker.idle = true
	}()

	received, err := decrypt((decodeBase64(string(job.Data()))), worker.key, worker.config.encryption)
	if err != nil {
		logger.Errorf("decrypt failed: %s", err.Error())
		return nil, nil
	}
	taskCounter.WithLabelValues(received.typ).Inc()
	worker.mainWorker.tasks++

	logger.Tracef("job data: %s", received)

	result := readAndExecute(received, worker.key, worker.config)

	if result.returnCode > 0 {
		errorCounter.WithLabelValues(received.typ).Inc()
	}

	if received.resultQueue != "" {
		var sendSuccess bool
		// send result back to any server
		for _, address := range worker.config.server {
			sendSuccess = sendAnswer(result, worker.key, address, worker.config.encryption)
			if sendSuccess {
				break
			}
		}

		// send to duplicate servers as well
		for _, dupAddress := range worker.config.dupserver {
			if worker.config.dupResultsArePassive {
				result.active = "passive"
			}
			sendSuccess = sendAnswer(result, worker.key, dupAddress, worker.config.encryption)
			if sendSuccess {
				break
			}
		}
	}
	return nil, nil
}

//Shutdown and unregister this worker
func (worker *worker) Shutdown() {
	logger.Debugf("shutting down")
	if worker.worker != nil {
		worker.worker.ErrorHandler = nil
		worker.worker.Shutdown()
	}
	worker.worker = nil
	worker.mainWorker.unregisterWorker(worker)
}
