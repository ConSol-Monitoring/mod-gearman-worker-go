package modgearman

import (
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	libworker "github.com/appscode/g2/worker"
)

type worker struct {
	id         string
	what       string
	worker     *libworker.Worker
	activeJobs int
	config     *configurationStruct
	mainWorker *mainWorker
	jobs       []*receivedStruct
	lock       sync.RWMutex
}

// creates a new worker and returns a pointer to it
func newWorker(what string, configuration *configurationStruct, mainWorker *mainWorker) *worker {
	logger.Tracef("starting new %sworker", what)
	worker := &worker{
		what:       what,
		activeJobs: 0,
		config:     configuration,
		mainWorker: mainWorker,
	}
	worker.id = fmt.Sprintf("%p", worker)

	w := libworker.New(libworker.OneByOne)
	worker.worker = w

	w.ErrorHandler = func(e error) {
		worker.errorHandler(e)
	}

	worker.registerFunctions(configuration)

	// listen to this servers
	servers := mainWorker.ActiveServerList()
	if len(servers) == 0 {
		return nil
	}
	for _, address := range servers {
		status := worker.mainWorker.GetServerStatus(address)
		if status != "" {
			continue
		}
		err := w.AddServer("tcp", address)
		if err != nil {
			worker.mainWorker.SetServerStatus(address, err.Error())
			return nil
		}
	}

	// check if worker is ready
	if err := w.Ready(); err != nil {
		logger.Debugf("worker not ready closing again: %w", err)
		worker.Shutdown()
		return nil
	}

	// start the worker
	go func() {
		defer logPanicExit()
		w.Work()
	}()

	return worker
}

func (worker *worker) registerFunctions(configuration *configurationStruct) {
	w := worker.worker
	// specifies what events the worker listens
	switch worker.what {
	case "check":
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

		// register for the hostgroups
		if len(worker.config.hostgroups) > 0 {
			for _, element := range worker.config.hostgroups {
				w.AddFunc("hostgroup_"+element, worker.doWork, libworker.Unlimited)
			}
		}

		// register for servicegroups
		if len(worker.config.servicegroups) > 0 {
			for _, element := range worker.config.servicegroups {
				w.AddFunc("servicegroup_"+element, worker.doWork, libworker.Unlimited)
			}
		}
	case "status":
		statusQueue := fmt.Sprintf("worker_%s", configuration.identifier)
		w.AddFunc(statusQueue, worker.returnStatus, libworker.Unlimited)
	default:
		logger.Panicf("type not implemented: %s", worker.what)
	}
}

func (worker *worker) doWork(job libworker.Job) (res []byte, err error) {
	res = []byte("OK")
	logger.Tracef("worker got a job: %s", job.Handle())

	worker.activeJobs++

	received, err := decrypt((decodeBase64(string(job.Data()))), worker.config.encryption)
	if err != nil {
		logger.Errorf("decrypt failed: %w", err)
		worker.activeJobs--
		return
	}
	worker.mainWorker.tasks++

	logJob(job, received, "incoming", nil)
	logger.Trace(received)

	worker.addJob(received)
	defer worker.removeJob(received)

	if !worker.considerballooning() {
		answer := worker.executeJob(received)
		worker.activeJobs--
		if received.Canceled {
			logJob(job, received, "canceled", answer)
			res = make([]byte, 0)
			return res, fmt.Errorf("job has been canceled")
		}
		logJob(job, received, "finished", answer)
		return
	}

	finChan := make(chan bool, 1)
	go func() {
		defer logPanicExit()
		defer func() {
			worker.activeJobs--
			if received.ballooning {
				worker.mainWorker.curBallooningWorker--
				ballooningWorkerCount.Set(float64(worker.mainWorker.curBallooningWorker))
			}
			finChan <- true
		}()
		answer := worker.executeJob(received)
		if received.Canceled {
			logJob(job, received, "canceled", answer)
		} else {
			logJob(job, received, "finished", answer)
		}
	}()

	ticker := time.NewTicker(time.Duration(worker.config.backgroundingThreshold) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-finChan:
			return
		case <-ticker.C:
			// check again if are there open files left for ballooning
			if worker.startballooning() {
				logger.Debugf("job: %s runs for more than %d seconds, backgrounding...", job.Handle(), worker.config.backgroundingThreshold)
				worker.mainWorker.curBallooningWorker++
				ballooningWorkerCount.Set(float64(worker.mainWorker.curBallooningWorker))
				received.ballooning = true
				return
			}
		}
	}
}

// considerballooning returns true if ballooning is enabled and threshold is reached
func (worker *worker) considerballooning() bool {
	if worker.config.backgroundingThreshold <= 0 {
		return false
	}

	// only if 70% of our workers are utilized
	if worker.mainWorker.workerUtilization < ballooningUtilizationThreshold {
		return false
	}

	return true
}

// startballooning returns true if conditions for ballooning are met (backgrounding jobs after backgroundingThreshold of seconds)
func (worker *worker) startballooning() bool {
	if worker.config.backgroundingThreshold <= 0 {
		return false
	}

	if !worker.mainWorker.checkLoads() {
		return false
	}

	if !worker.mainWorker.checkMemory() {
		return false
	}

	// only if 70% of our workers are utilized
	if worker.mainWorker.workerUtilization < ballooningUtilizationThreshold {
		return false
	}

	// are there open files left for ballooning
	if worker.mainWorker.curBallooningWorker >= (worker.mainWorker.maxPossibleWorker - worker.config.maxWorker) {
		return false
	}

	logger.Debugf("ballooning: cur: %d max: %d", worker.mainWorker.curBallooningWorker, (worker.mainWorker.maxPossibleWorker - worker.config.maxWorker))
	return true
}

// executeJob executes the job and handles sending the result
func (worker *worker) executeJob(received *receivedStruct) *answer {
	result := readAndExecute(received, worker.config)

	if !received.Canceled && received.resultQueue != "" {
		logger.Tracef("result:\n%s", result)
		enqueueServerResult(result)
		enqueueDupServerResult(worker.config, result)
	}

	return result
}

// errorHandler gets called if the libworker worker throws an error
func (worker *worker) errorHandler(e error) {
	switch err := e.(type) {
	case *libworker.WorkerDisconnectError:
		_, addr := err.Server()
		logger.Debugf("worker disconnect: %w from %s", e, addr)
		worker.mainWorker.SetServerStatus(addr, err.Error())
	default:
		logger.Errorf("worker error: %w", e)
		logger.Errorf("%s", debug.Stack())
	}
	worker.Shutdown()
}

// Shutdown and deregister this worker
func (worker *worker) Shutdown() {
	logger.Debugf("worker shutting down")
	defer func() {
		if worker.mainWorker != nil && worker.mainWorker.running {
			worker.mainWorker.unregisterWorker(worker)
		}
	}()
	if worker.worker != nil {
		worker.worker.ErrorHandler = nil
		if worker.activeJobs > 0 {
			// try to stop gracefully
			worker.worker.Shutdown()
		}
		if worker.worker != nil {
			worker.worker.Close()
		}
	}
	worker.worker = nil
}

// Cancel current job(s)
func (worker *worker) Cancel() {
	if worker.activeJobs == 0 {
		return
	}
	logger.Debugf("worker %s cancling current jobs", worker.id)
	worker.lock.Lock()
	for _, j := range worker.jobs {
		if j.Cancel != nil {
			j.Cancel()
		}
	}
	worker.lock.Unlock()
}

func logJob(job libworker.Job, received *receivedStruct, prefix string, result *answer) {
	suffix := ""
	if result != nil {
		suffix = fmt.Sprintf(" (took: %.3fs | exec: %s)", result.finishTime-result.startTime, result.execType)
	}
	switch {
	case received.serviceDescription != "":
		logger.Debugf("%s %-13s job: handle: %s - host: %20s - service: %s%s", prefix, received.typ, job.Handle(), received.hostName, received.serviceDescription, suffix)
	case received.hostName != "":
		logger.Debugf("%s %-13s job: handle: %s - host: %20s%s", prefix, received.typ, job.Handle(), received.hostName, suffix)
	default:
		logger.Debugf("%s %-13s job: handle: %s%s", prefix, received.typ, job.Handle(), suffix)
	}
}

func (worker *worker) addJob(received *receivedStruct) {
	worker.lock.Lock()
	worker.jobs = append(worker.jobs, received)
	worker.lock.Unlock()
}

func (worker *worker) removeJob(received *receivedStruct) {
	worker.lock.Lock()
	for i, j := range worker.jobs {
		if j == received {
			worker.jobs = append(worker.jobs[:i], worker.jobs[i+1:]...)
		}
	}
	worker.lock.Unlock()
}
