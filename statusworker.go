package main

import (
	"fmt"
	"runtime/debug"

	libworker "github.com/appscode/g2/worker"
)

//creates a new worker and returns a pointer to it
// counterChanel will receive +1 if a job is received and started
// and -1 if a job is completed
func newStatusWorker(configuration *configurationStruct, mainWorker *mainWorker) *worker {
	logger.Tracef("starting new status worker")
	worker := &worker{
		config:     configuration,
		mainWorker: mainWorker,
	}

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
	statusQueue := fmt.Sprintf("worker_%s", configuration.identifier)
	w.AddFunc(statusQueue, worker.returnStatus, libworker.Unlimited)

	//check if worker is ready
	if err := w.Ready(); err != nil {
		logger.Debugf("worker not ready closing again: %s", err.Error())
		return nil
	}
	//start the worker
	go func() {
		defer logPanicExit()
		w.Work()
	}()

	return worker
}

func (worker *worker) returnStatus(job libworker.Job) (result []byte, err error) {
	logger.Debugf("status worker got a job: %s", job.Handle())

	received := string(job.Data())
	logger.Tracef("job data: %s", received)

	result = []byte(fmt.Sprintf("%s has %d worker and is working on %d jobs. Version: %s|worker=%d;;;%d;%d jobs=%dc",
		worker.config.identifier,
		len(worker.mainWorker.workerMap),
		worker.mainWorker.activeWorkers,
		VERSION,
		len(worker.mainWorker.workerMap),
		worker.config.minWorker,
		worker.config.maxWorker,
		worker.mainWorker.tasks,
	))

	return
}
