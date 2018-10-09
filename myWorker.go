package main

import (
	"log"
	"runtime/debug"
	time "time"

	libworker "github.com/appscode/g2/worker"
)

type worker struct {
	worker    *libworker.Worker
	idle      bool
	idleSince time.Time
	maxJobs   int
	start     chan int
	timer     *time.Timer
}

//creates a new worker and returns a pointer to it
// counterChanel will receive +1 if a job is received and started
// and -1 if a job is completed
func newWorker(counterChanel chan int) *worker {
	workerCount.Inc()
	iddleWorkerCount.Inc()
	worker := &worker{
		maxJobs:   config.max_jobs,
		idle:      true,
		idleSince: time.Now(),
		start:     counterChanel}
	//create the libWorker

	w := libworker.New(libworker.OneByOne)
	defer w.Close()

	w.ErrorHandler = func(e error) {
<<<<<<< 8ca1a00a87b214a983dab743ed1a0d88c22b21e4
		logger.Errorf(e.Error())
		logger.Errorf("%s", debug.Stack())
=======
		logger.Error(e)

>>>>>>> fixed the benchmark test
	}

	//listen to this servers
	for _, address := range config.server {
		err := w.AddServer("tcp4", address)
		if err != nil {
			logger.Error(err)
		}
	}

	// specifies what events the worker listens
	if config.eventhandler {
		w.AddFunc("eventhandler", worker.doWork, libworker.Unlimited)
	}
	if config.hosts {
		w.AddFunc("host", worker.doWork, libworker.Unlimited)
	}
	if config.services {
		w.AddFunc("service", worker.doWork, libworker.Unlimited)
	}
	if config.notifications {
		w.AddFunc("notification", worker.doWork, libworker.Unlimited)
	}

	//register for the hostgroups
	if len(config.hostgroups) > 0 {
		for _, element := range config.hostgroups {
			w.AddFunc("hostgroup_"+element, worker.doWork, libworker.Unlimited)
		}
	}

	//register for servicegroups
	if len(config.servicegroups) > 0 {
		for _, element := range config.servicegroups {
			w.AddFunc("servicegroup_"+element, worker.doWork, libworker.Unlimited)
		}
	}

	//check if worker is ready
	if err := w.Ready(); err != nil {
		log.Fatal(err)
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

	//stop the idle timeout timer
	worker.timer.Stop()
	//set worker to idle and idleSince back to zero
	worker.idle = false
	worker.idleSince = time.Now()
	worker.start <- 1

	taskCounter.Inc()
	iddleWorkerCount.Dec()
	workingWorkerCount.Inc()

	received := decrypt((decodeBase64(string(job.Data()))), key)

	result := readAndExecute(received, key)

	if received.result_queue != "" {

		var sendSuccess bool
		//send to all servers
		for _, address := range config.server {
			sendSuccess = sendAnswer(result, key, address)
		}

		//if failed send to al duplicate servers
		//send to al servers
		if !sendSuccess {
			for _, dupAddress := range config.dupserver {
				if config.dup_results_are_passive {
					result.active = "passive"
				}
				sendSuccess = sendAnswer(result, key, dupAddress)
			}
		}
	}
	iddleWorkerCount.Inc()
	workingWorkerCount.Dec()

	//set back to idling
	worker.start <- -1
	worker.idle = true
	worker.idleSince = time.Now()

	worker.maxJobs--
	if worker.maxJobs < 1 {
		worker.closeWorker()
	}

	//start the timer again
	worker.startIdleTimer()
	return nil, nil
}

//starts the idle timer, after the time from the config file timeout() gets called
//if a job is received the stop call on worker.time stops the timer
func (worker *worker) startIdleTimer() {
	worker.timer = time.AfterFunc(time.Duration(config.idle_timeout)*time.Second, worker.timeout)
}

//after the max idle time has passed we try to remove the worker
func (worker *worker) timeout() {
	removeWorker(worker)
}

//everything needed to stop the worker without
//creating any memory leaks
func (worker *worker) close() {
	//close only if more than the minimum workers are available
	removeWorker(worker)
}

func (worker *worker) closeWorker() {
	worker.worker.Close()
	worker.worker = nil
	workerCount.Desc()
}
