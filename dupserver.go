package modgearman

import (
	time "time"

	"github.com/appscode/g2/client"
)

var dupjobsToSendPerServer = make(map[string](chan *answer))

func initialiseDupServerConsumers(config *configurationStruct) {
	if len(config.dupserver) > 0 {
		for _, dupAddress := range config.dupserver {
			logger.Debugf("creating dupserverConsumer for: %s", dupAddress)
			dupjobsToSendPerServer[dupAddress] = make(chan *answer, config.dupServerBacklogQueueSize)
			go runDupServerConsumer(dupAddress, dupjobsToSendPerServer[dupAddress], config)
		}
	}
}

func runDupServerConsumer(dupAddress string, channel chan *answer, config *configurationStruct) {
	for {
		item := <-channel
		for {
			error := sendResultDup(item, dupAddress, config)
			if error == nil {
				break
			} else {
				logger.Debugf("failed to send back result (to dupserver): %s", error.Error())
				time.Sleep(5 * time.Second)
			}
		}
	}
}

func sendResultDup(item *answer, dupAddress string, config *configurationStruct) error {
	var err error
	var client *client.Client

	if config.dupResultsArePassive {
		item.active = "passive"
	}

	client, err = sendAnswer(client, item, dupAddress, config.encryption)

	if client != nil {
		client.Close()
	}

	return err
}

func enqueueDupServerResult(config *configurationStruct, result *answer) {
	for _, dupAddress := range config.dupserver {
		var channel = dupjobsToSendPerServer[dupAddress]
		select {
		case channel <- result:
		default:
			logger.Debugf("channel is at capacity (%d), dropping message (to dupserver): %s", config.dupServerBacklogQueueSize, dupAddress)
		}
	}
}
