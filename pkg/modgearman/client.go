package modgearman

import (
	"fmt"
	"os"

	"github.com/appscode/g2/client"
	"github.com/appscode/g2/pkg/runtime"
)

/**
*@input answer: the struct containing the data to be sent
*@input server: the server:port address where to send the data
 */
func sendAnswer(currentClient *client.Client, answer *answer, server string, encrypted bool) (*client.Client, error) {
	if currentClient == nil {
		cl1, err := client.New("tcp", server)
		if err != nil {
			return nil, fmt.Errorf("client: %w", err)
		}
		currentClient = cl1
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
	cl1, err := client.New("tcp", args.Host)
	if err != nil {
		err = fmt.Errorf("%s UNKNOWN - cannot create gearman client", pluginName)

		return "", err
	}
	defer cl1.Close()

	ret, taskErr := cl1.DoBg(args.Queue, []byte(args.TextToSend), runtime.JobHigh)
	if taskErr != nil {
		taskErr = fmt.Errorf("%w", taskErr)

		return "", taskErr
	}

	return ret, nil
}

func sendWorkerJob(args *CheckGmArgs) (string, error) {
	cl1, err := client.New("tcp", args.Host)
	if err != nil {
		return "", fmt.Errorf("%s UNKNOWN - cannot create gearman client", pluginName)
	}
	defer cl1.Close()

	ansChan := make(chan string)

	jobHandler := func(resp *client.Response) {
		data, err := resp.Result()
		ansChan <- string(data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error, %s\n", err)

			return
		}
	}

	_, taskErr := cl1.Do(args.Queue, []byte(args.TextToSend), runtime.JobHigh, jobHandler)
	if taskErr != nil {
		taskErr = fmt.Errorf("%w", taskErr)

		return "", taskErr
	}
	response := <-ansChan

	return response, nil
}
