package modgearman

import (
	"errors"
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
	config     *config
	mainWorker *mainWorker
	jobs       []*request
	lock       sync.RWMutex
}

// creates a new worker and returns a pointer to it
func newWorker(what string, configuration *config, mainWorker *mainWorker) *worker {
	log.Tracef("starting new %sworker", what)
	worker := &worker{
		what:       what,
		activeJobs: 0,
		config:     configuration,
		mainWorker: mainWorker,
	}
	worker.id = fmt.Sprintf("%p", worker)

	wrk := libworker.New(libworker.OneByOne)
	worker.worker = wrk

	wrk.ErrorHandler = func(e error) {
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
		err := wrk.AddServer("tcp", address)
		if err != nil {
			worker.mainWorker.SetServerStatus(address, err.Error())

			return nil
		}
	}

	// check if worker is ready
	if err := wrk.Ready(); err != nil {
		log.Debugf("worker not ready closing again: %w", err)
		worker.Shutdown()

		return nil
	}

	// start the worker
	go func() {
		defer logPanicExit()
		wrk.Work()
	}()

	return worker
}

func (worker *worker) registerFunctions(configuration *config) {
	wrk := worker.worker
	// specifies what events the worker listens
	switch worker.what {
	case "check":
		if worker.config.eventhandler {
			logError(wrk.AddFunc("eventhandler", worker.doWork, libworker.Unlimited))
		}
		if worker.config.hosts {
			logError(wrk.AddFunc("host", worker.doWork, libworker.Unlimited))
		}
		if worker.config.services {
			logError(wrk.AddFunc("service", worker.doWork, libworker.Unlimited))
		}
		if worker.config.notifications {
			logError(wrk.AddFunc("notification", worker.doWork, libworker.Unlimited))
		}

		// register for the hostgroups
		if len(worker.config.hostgroups) > 0 {
			for _, element := range worker.config.hostgroups {
				logError(wrk.AddFunc("hostgroup_"+element, worker.doWork, libworker.Unlimited))
			}
		}

		// register for servicegroups
		if len(worker.config.servicegroups) > 0 {
			for _, element := range worker.config.servicegroups {
				logError(wrk.AddFunc("servicegroup_"+element, worker.doWork, libworker.Unlimited))
			}
		}
	case "status":
		statusQueue := fmt.Sprintf("worker_%s", configuration.identifier)
		logError(wrk.AddFunc(statusQueue, worker.returnStatus, libworker.Unlimited))
	default:
		log.Panicf("type not implemented: %s", worker.what)
	}
}

func (worker *worker) doWork(job libworker.Job) (res []byte, err error) {
	defer logPanicExit()

	res = []byte("OK")
	log.Tracef("worker got a job: %s", job.Handle())

	worker.activeJobs++
	received, err := decryptJobData(job.Data(), worker.config.encryption)
	if err != nil {
		log.Errorf("decrypt failed: %w", err)
		worker.activeJobs--

		return nil, err
	}

	worker.mainWorker.tasks++
	logJob(job, received, "incoming", nil)
	log.Trace(received)

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

		return res, nil
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
			return res, nil
		case <-ticker.C:
			// check again if are there open files left for ballooning
			if worker.startballooning() {
				log.Debugf("job: %s runs for more than %d seconds, backgrounding...",
					job.Handle(), worker.config.backgroundingThreshold)
				worker.mainWorker.curBallooningWorker++
				ballooningWorkerCount.Set(float64(worker.mainWorker.curBallooningWorker))
				received.ballooning = true

				return res, nil
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

// startballooning returns true if conditions for ballooning are met
// (backgrounding jobs after backgroundingThreshold of seconds)
func (worker *worker) startballooning() bool {
	if worker.config.backgroundingThreshold <= 0 {
		return false
	}

	passed, _ := worker.mainWorker.checkLoads()
	if !passed {
		return false
	}

	passed, _ = worker.mainWorker.checkMemory()
	if !passed {
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

	log.Debugf("ballooning: cur: %d max: %d",
		worker.mainWorker.curBallooningWorker, (worker.mainWorker.maxPossibleWorker - worker.config.maxWorker))

	return true
}

// executeJob executes the job and handles sending the result
func (worker *worker) executeJob(received *request) *answer {
	result := readAndExecute(received, worker.config)

	if !received.Canceled && received.resultQueue != "" {
		log.Tracef("result:\n%s", result)
		enqueueServerResult(result)
		enqueueDupServerResult(worker.config, result)
	}

	return result
}

// errorHandler gets called if the libworker worker throws an error
func (worker *worker) errorHandler(err error) {
	var discoErr *libworker.WorkerDisconnectError
	if errors.As(err, &discoErr) {
		_, addr := discoErr.Server()
		log.Debugf("worker disconnect: %w from %s", err, addr)
		worker.mainWorker.SetServerStatus(addr, discoErr.Error())
	} else {
		log.Errorf("worker error: %w: %s", err, err.Error())
		log.Errorf("%s", debug.Stack())
	}
	worker.Shutdown()
}

// Shutdown and deregister this worker
func (worker *worker) Shutdown() {
	log.Debugf("worker shutting down")
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
	log.Debugf("worker %s cancling current jobs", worker.id)
	worker.lock.Lock()
	for _, j := range worker.jobs {
		if j.Cancel == nil {
			continue
		}
		j.Cancel()
	}
	if worker.worker != nil {
		worker.worker.Close()
	}
	worker.lock.Unlock()
}

func logJob(job libworker.Job, received *request, prefix string, result *answer) {
	suffix := ""
	if result != nil {
		suffix = fmt.Sprintf(" (took: %.3fs | rc: %d | exec: %s)",
			result.finishTime-result.startTime, result.returnCode, result.execType)
	}
	switch {
	case received.serviceDescription != "":
		log.Debugf("%s %-7s - handle: %s - host: %20s - service: %s%s",
			prefix, received.typ, job.Handle(), received.hostName, received.serviceDescription, suffix)
	case received.hostName != "":
		log.Debugf("%s %-7s - handle: %s - host: %20s%s", prefix, received.typ, job.Handle(), received.hostName, suffix)
	default:
		log.Debugf("%s %-7s - handle: %s%s", prefix, received.typ, job.Handle(), suffix)
		if result != nil && log.IsV(2) {
			log.Tracef("Output:\n%s", result.output)
		}
	}
}

func (worker *worker) addJob(received *request) {
	worker.lock.Lock()
	worker.jobs = append(worker.jobs, received)
	worker.lock.Unlock()
}

func (worker *worker) removeJob(received *request) {
	worker.lock.Lock()
	for i, j := range worker.jobs {
		if j == received {
			worker.jobs = append(worker.jobs[:i], worker.jobs[i+1:]...)
		}
	}
	worker.lock.Unlock()
}
