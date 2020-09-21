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

var dupjobsToSendPerServer = map[string]*safelist{}

func runDupServerConsumer(dupAddress string, list *safelist, config *configurationStruct) {
	for {
		list.mutex.Lock()
		item := list.list.Front()
		list.mutex.Unlock()

		if item != nil {
			logger.Debugf("pushing item: %i", item)

			var error = sendResultDup(item.Value.(*answer), dupAddress, config)
			if error != nil {
				logger.Debugf("failed to send back result (to dupserver): %s", error.Error())
				time.Sleep(1 * time.Second)
				continue
			}
			list.mutex.Lock()
			list.list.Remove(item)
			list.mutex.Unlock()
		} else {
			time.Sleep(1 * time.Second)
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

func enQueueDupserver(config *configurationStruct, result *answer) {
	for _, dupAddress := range config.dupserver {
		var safeList = dupjobsToSendPerServer[dupAddress]
		safeList.mutex.Lock()
		safeList.list.PushBack(result)
		for {
			if safeList.list.Len() <= config.maxNumberOfAsyncRequests {
				break
			}

			var item = safeList.list.Front()
			safeList.list.Remove(item)
		}
		safeList.mutex.Unlock()
	}
}
