package modgearman

import (
	"strings"
	time "time"

	"github.com/appscode/g2/client"
)

type resultServerConsumer struct {
	queue              chan *answer
	terminationRequest chan bool
	config             *configurationStruct
}

var (
	resultServerConsumers []*resultServerConsumer
	resultServerQueue     chan *answer
)

func initializeResultServerConsumers(config *configurationStruct) {
	numResultServer := ResultServerWorker
	if numResultServer > config.maxWorker {
		numResultServer = config.maxWorker
	}

	logger.Debugf("creating %d result worker for: [%s]", numResultServer, strings.Join(config.server, ", "))
	resultServerConsumers = make([]*resultServerConsumer, 0, numResultServer)

	// all result worker share one queue
	resultServerQueue = make(chan *answer, ResultServerQueueSize) // queue at least 1k results before stalling

	// create 10 result worker
	for len(resultServerConsumers) < numResultServer {
		consumer := &resultServerConsumer{
			terminationRequest: make(chan bool),
			queue:              resultServerQueue,
			config:             config,
		}
		go runResultServerConsumer(consumer)

		resultServerConsumers = append(resultServerConsumers, consumer)
	}
}

func terminateResultServerConsumers() bool {
	logger.Debugf("Terminating ResultServers")
	for i := range resultServerConsumers {
		resultServerConsumers[i].terminationRequest <- true
	}
	resultServerConsumers = nil
	return true
}

func runResultServerConsumer(server *resultServerConsumer) {
	var curClient *client.Client
	for {
		var result *answer
		select {
		case <-server.terminationRequest:
			if curClient != nil {
				curClient.Close()
			}
			return
		case result = <-server.queue:
			var err error
			var sendSuccess bool
			var shouldExit bool
			shouldExit, sendSuccess, curClient, err = sendResult(server, curClient, result)

			if !sendSuccess && err != nil {
				logger.Errorf("failed to send back result: %w", err)
			}
			if shouldExit {
				return
			}
			break
		}
	}
}

func sendResult(server *resultServerConsumer, curClient *client.Client, result *answer) (shouldExit bool, sendSuccess bool, retClient *client.Client, err error) {
	// send result back to any server
	sendSuccess = false
	retries := 0
	for {
		var c *client.Client
		for _, address := range server.config.server {
			c, err = sendAnswer(curClient, result, address, server.config.encryption)
			if err == nil {
				curClient = c
				sendSuccess = true
				break
			}
			curClient = nil
			if c != nil {
				c.Close()
			}
		}
		if sendSuccess || retries > 120 {
			break
		}
		if err != nil {
			if retries == 30 {
				logger.Warnf("failed to send back result, will continue to retry for 2 minutes: %w", err)
			} else {
				logger.Tracef("still failing to send back result: %w", err)
			}
		}
		retries++
		select {
		case <-server.terminationRequest:
			if curClient != nil {
				curClient.Close()
			}
			shouldExit = true
			return
		default:
			time.Sleep(1 * time.Second)
			continue
		}
	}
	retClient = curClient
	return
}

func enqueueServerResult(result *answer) {
	// since it is a shared queue, we simply use the first one
	resultServerQueue <- result
}
