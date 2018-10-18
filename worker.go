package modgearman

import (
	"fmt"
	"runtime/debug"
	"time"

	"github.com/appscode/g2/client"
	libworker "github.com/appscode/g2/worker"
)

type worker struct {
	id         string
	worker     *libworker.Worker
	idle       bool
	start      chan int
	config     *configurationStruct
	mainWorker *mainWorker
	tasks      int
	client     *client.Client
	dupclient  *client.Client
}

//creates a new worker and returns a pointer to it
// counterChanel will receive +1 if a job is received and started
// and -1 if a job is completed
func newWorker(counterChanel chan int, configuration *configurationStruct, mainWorker *mainWorker) *worker {
	logger.Tracef("starting new worker")
	worker := &worker{
		idle:       true,
		start:      counterChanel,
		config:     configuration,
		mainWorker: mainWorker,
		client:     nil,
		dupclient:  nil,
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
		err := w.AddServer("tcp", address)
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

func (worker *worker) doWork(job libworker.Job) (res []byte, err error) {
	res = []byte("")
	logger.Debugf("worker got a job: %s", job.Handle())

	//set worker to idle
	worker.idle = false
	worker.start <- 1

	defer func() {
		worker.start <- -1
		worker.idle = true
	}()

	received, err := decrypt((decodeBase64(string(job.Data()))), worker.config.encryption)
	if err != nil {
		logger.Errorf("decrypt failed: %s", err.Error())
		return
	}
	taskCounter.WithLabelValues(received.typ).Inc()
	worker.mainWorker.tasks++

	logger.Tracef("job data: %s", received)

	result := readAndExecute(received, worker.config)

	if result.returnCode > 0 {
		errorCounter.WithLabelValues(received.typ).Inc()
	}

	if received.resultQueue != "" {
		worker.SendResult(result)
		worker.SendResultDup(result)
	}
	return
}

//SendResult sends the result back to the result queue
func (worker *worker) SendResult(result *answer) {
	// send result back to any server
	sendSuccess := false
	retries := 0
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
			if c != nil {
				c.Close()
			}
		}
		if sendSuccess || retries > 120 {
			break
		}
		if retries == 0 && err != nil {
			logger.Errorf("failed to send back result, will continue to retry for 2 minutes: %s", err.Error())
		}
		time.Sleep(1 * time.Second)
		retries++
	}
}

func (worker *worker) SendResultDup(result *answer) {
	if len(worker.config.dupserver) == 0 {
		return
	}
	// send to duplicate servers as well
	sendSuccess := false
	retries := 0
	for {
		var err error
		var c *client.Client
		for _, dupAddress := range worker.config.dupserver {
			if worker.config.dupResultsArePassive {
				result.active = "passive"
			}
			c, err = sendAnswer(worker.dupclient, result, dupAddress, worker.config.encryption)
			if err == nil {
				worker.dupclient = c
				sendSuccess = true
				break
			}
			if c != nil {
				c.Close()
			}
		}
		if sendSuccess || retries > 120 {
			break
		}
		if retries == 0 && err != nil {
			logger.Errorf("failed to send back result (to dupserver), will continue to retry for 2 minutes: %s", err.Error())
		}
		time.Sleep(1 * time.Second)
		retries++
	}
}

//Shutdown and unregister this worker
func (worker *worker) Shutdown() {
	logger.Debugf("worker shutting down")
	if worker.worker != nil {
		worker.worker.ErrorHandler = nil
		if !worker.idle {
			// try to stop gracefully
			worker.worker.Shutdown()
		}
		worker.worker.Close()
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
	worker.mainWorker.unregisterWorker(worker)
}
