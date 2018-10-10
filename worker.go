package main

import (
	"runtime/debug"
	time "time"

	libworker "github.com/appscode/g2/worker"
)

type worker struct {
	worker     *libworker.Worker
	idle       bool
	idleSince  time.Time
	maxJobs    int
	start      chan int
	timer      *time.Timer
	config     *configurationStruct
	key        []byte
	mainWorker *mainWorker
}

//creates a new worker and returns a pointer to it
// counterChanel will receive +1 if a job is received and started
// and -1 if a job is completed
func newWorker(counterChanel chan int, configuration *configurationStruct, key []byte, mainWorker *mainWorker) *worker {
	logger.Debugf("starting new worker")
	workerCount.Inc()
	idleWorkerCount.Inc()
	worker := &worker{
		maxJobs:    configuration.maxJobs,
		idle:       true,
		idleSince:  time.Now(),
		start:      counterChanel,
		config:     configuration,
		key:        key,
		mainWorker: mainWorker}
	//create the libWorker

	w := libworker.New(libworker.OneByOne)
	defer w.Close()

	w.ErrorHandler = func(e error) {
		logger.Errorf(e.Error())
		logger.Errorf("%s", debug.Stack())
		worker.close()
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
		//logger.Fatal(err)
		// logger.Debug("worker not ready closing again")
		// worker.close()
		worker.mainWorker.removeFromSlice(worker)
		return nil
	}
	//start the worker
	go func() {
		defer logPanicExit()
		w.Work()
	}()
	//start the idle
	worker.startIdleTimer()

	worker.worker = w
	return worker
}

func (worker *worker) doWork(job libworker.Job) ([]byte, error) {
	logger.Debugf("worker got a job: %s", job.Handle())

	//stop the idle timeout timer
	worker.timer.Stop()
	//set worker to idle and idleSince back to zero
	worker.idle = false
	worker.idleSince = time.Now()
	worker.start <- 1

	idleWorkerCount.Dec()
	workingWorkerCount.Inc()

	received := decrypt((decodeBase64(string(job.Data()))), worker.key, worker.config.encryption)
	taskCounter.WithLabelValues(received.typ).Inc()

	logger.Tracef("job data: %s", received)

	result := readAndExecute(received, worker.key, worker.config)

	if result.returnCode > 0 {
		errorCounter.WithLabelValues(received.typ).Inc()
	}

	if received.resultQueue != "" {

		var sendSuccess bool
		//send to all servers
		for _, address := range worker.config.server {
			sendSuccess = sendAnswer(result, worker.key, address, worker.config.encryption)
		}

		//if failed send to al duplicate servers
		//send to al servers
		if !sendSuccess {
			for _, dupAddress := range worker.config.dupserver {
				if worker.config.dupResultsArePassive {
					result.active = "passive"
				}
				sendSuccess = sendAnswer(result, worker.key, dupAddress, worker.config.encryption)
			}
		}
	}
	idleWorkerCount.Inc()
	workingWorkerCount.Dec()

	//set back to idling
	worker.start <- -1
	worker.idle = true
	worker.idleSince = time.Now()

	worker.maxJobs--
	if worker.maxJobs < 1 {
		worker.closeWorker()
		// worker.mainWorker.removeFromSlice(worker)
	}

	//start the timer again
	worker.startIdleTimer()
	return nil, nil
}

//starts the idle timer, after the time from the config file timeout() gets called
//if a job is received the stop call on worker.time stops the timer
func (worker *worker) startIdleTimer() {
	worker.timer = time.AfterFunc(time.Duration(worker.config.idleTimeout)*time.Second, worker.timeout)
}

//after the max idle time has passed we check if we can remove the worker
func (worker *worker) timeout() {
	if len(worker.mainWorker.workerSlice) < worker.config.minWorker {
		worker.mainWorker.removeWorker(worker)
	} else {
		worker.maxJobs = worker.config.maxJobs
		worker.idleSince = time.Now()
		worker.startIdleTimer()
	}
}

//everything needed to stop the worker without
//creating any memory leaks
func (worker *worker) close() {
	//close only if more than the minimum workers are available
	worker.mainWorker.removeWorker(worker)
}

func (worker *worker) closeWorker() {
	worker.worker.Close()
	worker.timer.Stop()
	worker.worker = nil
	workerCount.Desc()
}
