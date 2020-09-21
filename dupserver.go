package modgearman

import (
	"container/list"
	"sync"
	time "time"

	"github.com/appscode/g2/client"
)

type safelist struct {
	list  *list.List
	mutex sync.Mutex
}

var dupjobsToSendPerServer = make(map[string](chan *answer))

func initialiseDupServerConsumers(config *configurationStruct) {
	if len(config.dupserver) > 0 {
		for _, dupAddress := range config.dupserver {
			logger.Debugf("creating dupserverConsumer for: %s", dupAddress)
			dupjobsToSendPerServer[dupAddress] = make(chan *answer, config.maxNumberOfAsyncRequests)
			go runDupServerConsumer(dupAddress, dupjobsToSendPerServer[dupAddress], config)
		}
	}
}

func runDupServerConsumer(dupAddress string, channel chan *answer, config *configurationStruct) {
	for {
		item := <-channel
		var error = sendResultDup(item, dupAddress, config)
		if error != nil {
			logger.Debugf("failed to send back result (to dupserver): %s", error.Error())
			time.Sleep(1 * time.Second)
			continue
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

	if err != nil {
		logger.Debugf("failed to send back result (to dupserver): %s", err.Error())
	}
	return err
}

func enqueueDupServerResult(config *configurationStruct, result *answer) {
	for _, dupAddress := range config.dupserver {
		var channel = dupjobsToSendPerServer[dupAddress]
		if len(channel) < config.maxNumberOfAsyncRequests {
			channel <- result
		}
	}
}
