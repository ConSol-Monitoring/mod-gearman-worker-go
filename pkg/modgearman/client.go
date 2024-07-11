package modgearman

import (
	"fmt"
	"github.com/appscode/g2/client"
	"github.com/appscode/g2/pkg/runtime"
	"os"
)

/**
*@input answer: the struct containing the data to be sent
*@input server: the server:port address where to send the data
 */
func sendAnswer(currentClient *client.Client, answer *answer, server string, encrypted bool) (*client.Client, error) {
	if currentClient == nil {
		cl, err := client.New("tcp", server)
		if err != nil {
			return nil, fmt.Errorf("client: %w", err)
		}
		currentClient = cl
	}

	byteAnswer := createAnswer(answer, encrypted)

	// send the data in the background to the right queue
	_, err := currentClient.DoBg(answer.resultQueue, byteAnswer, runtime.JobNormal)
	if err != nil {
		return currentClient, fmt.Errorf("bgclient: %w", err)
	}

	return currentClient, nil
}

func sendWorkerJobBg(args *CheckGmArgs) (string, error) {
	cl, err := client.New("tcp", args.Host)
	if err != nil {
		err = fmt.Errorf("%s UNKNOWN - cannot create gearman client\n", pluginName)

		return "", err
	}
	defer cl.Close()

	ret, taskErr := cl.DoBg(args.Queue, []byte(args.TextToSend), runtime.JobHigh) //ToDo: Gebe hier Rückgabewert zurück wenn Verbose als option
	if taskErr != nil {
		return "", taskErr
	}

	return ret, nil
}

func sendWorkerJob(args *CheckGmArgs) (string, error) {
	cl, err := client.New("tcp", args.Host)
	if err != nil {
		return "", fmt.Errorf("%s UNKNOWN - cannot create gearman client\n", pluginName)
	}
	defer cl.Close()

	ansChan := make(chan string)

	jobHandler := func(resp *client.Response) {
		data, err := resp.Result()
		ansChan <- string(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error, %s\n", err)

			return
		}
	}

	_, taskErr := cl.Do(args.Queue, []byte(args.TextToSend), runtime.JobHigh, jobHandler) //ToDo: Gebe hier Rückgabewert zurück wenn Verbose als option
	if taskErr != nil {
		return "", taskErr
	}
	response := <-ansChan
	return response, nil
}
