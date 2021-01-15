package modgearman

import (
	time "time"

	"github.com/appscode/g2/client"
)

type dupServerConsumer struct {
	queue              chan *answer
	address            string
	terminationRequest chan bool
	config             *configurationStruct
}

var dupServerConsumers map[string]*dupServerConsumer

func initialiseDupServerConsumers(config *configurationStruct) {
	if len(config.dupserver) > 0 {
		dupServerConsumers = make(map[string]*dupServerConsumer)
		for _, dupAddress := range config.dupserver {
			logger.Debugf("creating dupserverConsumer for: %s", dupAddress)
			consumer := dupServerConsumer{
				terminationRequest: make(chan bool),
				queue:              make(chan *answer, config.dupServerBacklogQueueSize),
				address:            dupAddress,
				config:             config,
			}

			dupServerConsumers[dupAddress] = &consumer
			go runDupServerConsumer(consumer)
		}
	}
}

func terminateDupServerConsumers() bool {
	logger.Debugf("Terminating DupServers")
	for _, consumer := range dupServerConsumers {
		logger.Debugf("Sending DupServer TerminationRequest %s", consumer.address)
		consumer.terminationRequest <- true
		logger.Debugf("DupServer Terminated %s", consumer.address)
	}
	logger.Debugf("Completed all consumer termination")
	dupServerConsumers = nil
	return true
}

func runDupServerConsumer(dupServer dupServerConsumer) {
	var client *client.Client
	var item *answer
	var error error

	for {
		select {
		case <-dupServer.terminationRequest:
			return
		case item = <-dupServer.queue:
			for {
				client, error = sendResultDup(client, item, dupServer.address, dupServer.config)
				if error != nil {
					client = nil
					logger.Debugf("failed to send back result (to dupserver): %s", error.Error())
					select {
					case <-dupServer.terminationRequest:
						return
					default:
						time.Sleep(ConnectionRetryInterval * time.Second)
						continue
					}
				}
				break
			}
		}
	}
}

func sendResultDup(client *client.Client, item *answer, dupAddress string, config *configurationStruct) (*client.Client, error) {
	var error error

	if config.dupResultsArePassive {
		item.active = "passive"
	}

	client, error = sendAnswer(client, item, dupAddress, config.encryption)

	return client, error
}

func enqueueDupServerResult(config *configurationStruct, result *answer) {
	for _, dupAddress := range config.dupserver {
		channel := dupServerConsumers[dupAddress].queue
		select {
		case channel <- result:
		default:
			logger.Debugf("channel is at capacity (%d), dropping message (to dupserver): %s", config.dupServerBacklogQueueSize, dupAddress)
		}
	}
}
