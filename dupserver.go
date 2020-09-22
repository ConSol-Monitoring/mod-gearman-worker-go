package modgearman

import (
	time "time"

	"github.com/appscode/g2/client"
)

type dupServerConsumer struct {
	queue               chan *answer
	address             string
	terminationRequest  chan bool
	terminationResponse chan bool
	config              *configurationStruct
}

var dupServerConsumers = make(map[string]*dupServerConsumer)

func initialiseDupServerConsumers(config *configurationStruct) {
	if len(config.dupserver) > 0 {
		for _, dupAddress := range config.dupserver {
			logger.Debugf("creating dupserverConsumer for: %s", dupAddress)
			consumer := dupServerConsumer{
				terminationRequest:  make(chan bool),
				terminationResponse: make(chan bool),
				queue:               make(chan *answer, config.dupServerBacklogQueueSize),
				address:             dupAddress,
				config:              config,
			}

			dupServerConsumers[dupAddress] = &consumer
			go runDupServerConsumer(consumer)
		}
	}
}

func terminateDupServerConsumers() bool {
	for _, consumer := range dupServerConsumers {
		consumer.terminationRequest <- true
		<-consumer.terminationResponse
	}
	return true
}

func runDupServerConsumer(dupServer dupServerConsumer) {
	var client *client.Client
	var item *answer
	var error error

	for {
		if error == nil {
			select {
			case <-dupServer.terminationRequest:
				dupServer.terminationResponse <- true
				return
			case item = <-dupServer.queue:
			}
		}
		error := sendResultDup(client, item, dupServer.address, dupServer.config)
		if error != nil {
			client = nil
			logger.Debugf("failed to send back result (to dupserver): %s", error.Error())
			time.Sleep(ConnectionRetryInterval * time.Second)
		}
	}
}

func sendResultDup(client *client.Client, item *answer, dupAddress string, config *configurationStruct) error {
	var err error

	if config.dupResultsArePassive {
		item.active = "passive"
	}

	_, err = sendAnswer(client, item, dupAddress, config.encryption)

	return err
}

func enqueueDupServerResult(config *configurationStruct, result *answer) {
	for _, dupAddress := range config.dupserver {
		var channel = dupServerConsumers[dupAddress].queue
		select {
		case channel <- result:
		default:
			logger.Debugf("channel is at capacity (%d), dropping message (to dupserver): %s", config.dupServerBacklogQueueSize, dupAddress)
		}
	}
}
