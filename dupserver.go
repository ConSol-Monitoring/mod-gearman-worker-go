package modgearman

import (
	time "time"

	"github.com/appscode/g2/client"
)

var dupjobsToSendPerServer = make(map[string](chan *answer))
var terminationRequest map[string](chan bool)
var terminationResponse map[string](chan bool)
var dupServers []string

func initialiseDupServerConsumers(config *configurationStruct) {
	if len(config.dupserver) > 0 {
		terminationRequest = make(map[string](chan bool))
		terminationResponse = make(map[string](chan bool))
		dupServers = config.dupserver

		for _, dupAddress := range config.dupserver {
			logger.Debugf("creating dupserverConsumer for: %s", dupAddress)
			dupjobsToSendPerServer[dupAddress] = make(chan *answer, config.dupServerBacklogQueueSize)
			go runDupServerConsumer(dupAddress, dupjobsToSendPerServer[dupAddress], config)
		}
	}
}

func terminateDupServerConsumers() bool {
	for _, dupAddress := range dupServers {
		terminationRequest[dupAddress] <- true
		<-terminationResponse[dupAddress]
	}
	return true
}

func runDupServerConsumer(dupAddress string, channel chan *answer, config *configurationStruct) {
	var client *client.Client
	var item *answer
	var error error

	for {
		if error == nil {
			select {
			case <-terminationRequest[dupAddress]:
				terminationResponse[dupAddress] <- true
				return
			case item = <-channel:
			}
		}
		error := sendResultDup(client, item, dupAddress, config)
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
		var channel = dupjobsToSendPerServer[dupAddress]
		select {
		case channel <- result:
		default:
			logger.Debugf("channel is at capacity (%d), dropping message (to dupserver): %s", config.dupServerBacklogQueueSize, dupAddress)
		}
	}
}
