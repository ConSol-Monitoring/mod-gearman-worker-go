package modgearman

import (
	"container/list"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/appscode/g2/client"
	libworker "github.com/appscode/g2/worker"
)

type safelist struct {
	list  *list.List
	mutex sync.Mutex
}

var dupjobsToSendPerServer = map[string]safelist{}

type worker struct {
	id         string
	what       string
	worker     *libworker.Worker
	idle       bool
	config     *configurationStruct
	mainWorker *mainWorker
	client     *client.Client
	dupclient  *client.Client
}

// creates a new worker and returns a pointer to it
func newWorker(what string, configuration *configurationStruct, mainWorker *mainWorker) *worker {
	logger.Tracef("starting new %sworker", what)
	worker := &worker{
		what:       what,
		idle:       true,
		config:     configuration,
		mainWorker: mainWorker,
		client:     nil,
		dupclient:  nil,
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
		logger.Debugf("worker not ready closing again: %s", err.Error())
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
	res = []byte("")
	logger.Tracef("worker got a job: %s", job.Handle())

	worker.idle = false

	defer func() {
		worker.idle = true
	}()

	received, err := decrypt((decodeBase64(string(job.Data()))), worker.config.encryption)
	if err != nil {
		logger.Errorf("decrypt failed: %s", err.Error())
		return
	}
	taskCounter.WithLabelValues(received.typ).Inc()
	worker.mainWorker.tasks++

	logger.Debugf("incoming %s job: %s", received.typ, job.Handle())
	logger.Trace(received)

	result := readAndExecute(received, worker.config)

	if result.returnCode > 0 {
		errorCounter.WithLabelValues(received.typ).Inc()
	}

	if received.resultQueue != "" {

		logger.Tracef("result:\n%s", result)
		worker.SendResult(result)

		for _, dupAddress := range worker.config.dupserver {
			var safeList = dupjobsToSendPerServer[dupAddress]
			safeList.mutex.Lock()
			safeList.list.PushBack(result)
			safeList.mutex.Unlock()
		}

	}
	return
}

// errorHandler gets called if the libworker worker throws an error
func (worker *worker) errorHandler(e error) {
	switch err := e.(type) {
	case *libworker.WorkerDisconnectError:
		_, addr := err.Server()
		logger.Debugf("worker disconnect: %s from %s", e.Error(), addr)
		worker.mainWorker.SetServerStatus(addr, err.Error())
	default:
		logger.Errorf("worker error: %s", e.Error())
		logger.Errorf("%s", debug.Stack())
	}
	worker.Shutdown()
}

// SendResult sends the result back to the result queue
func (worker *worker) SendResult(result *answer) {
	// send result back to any server
	sendSuccess := false
	retries := 0
	var lastErr error
	for {
		var err error
		var c *client.Client
		for _, address := range worker.config.server {
			c, err = sendAnswer(worker.client, result, address, worker.config.encryption)
			if err == nil {
				worker.client = c
				sendSuccess = true
				break
			}
			worker.client = nil
			if c != nil {
				c.Close()
			}
		}
		if sendSuccess || retries > 120 {
			break
		}
		if err != nil {
			lastErr = err
			if retries == 0 {
				logger.Debugf("failed to send back result, will continue to retry for 2 minutes: %s", err.Error())
			} else {
				logger.Tracef("still failing to send back result: %s", err.Error())
			}
		}
		time.Sleep(1 * time.Second)
		retries++
	}
	if !sendSuccess && lastErr != nil {
		logger.Debugf("failed to send back result: %s", lastErr.Error())
	}
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
		if !worker.idle {
			// try to stop gracefully
			worker.worker.Shutdown()
		}
		if worker.worker != nil {
			worker.worker.Close()
		}
	}
	if worker.client != nil {
		worker.client.Close()
		worker.client = nil
	}
	if worker.dupclient != nil {
		worker.dupclient.Close()
		worker.dupclient = nil
	}
	worker.worker = nil
}
