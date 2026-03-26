package modgearman

import (
	"fmt"
	"os"
	"time"

	"github.com/appscode/g2/client"
	"github.com/appscode/g2/pkg/runtime"
)

const defaultClientTimeout = 10 * time.Second

func buildClient(server string) (*client.Client, error) {
	return buildClientWithTimeout(server, defaultClientTimeout)
}

func buildClientWithTimeout(server string, timeout time.Duration) (*client.Client, error) {
	clt, err := client.New("tcp", server)
	if err != nil {
		return nil, fmt.Errorf("client: %s", err.Error())
	}

	if timeout <= 0 {
		timeout = defaultClientTimeout
	}

	clt.ResponseTimeout = timeout

	return clt, nil
}

/**
*@input answer: the struct containing the data to be sent
*@input server: the server:port address where to send the data
 */
func sendAnswer(currentClient *client.Client, answer *answer, server string, encrypted bool, timeout time.Duration) (*client.Client, error) {
	if currentClient == nil {
		cl1, err := buildClientWithTimeout(server, timeout)
		if err != nil {
			return nil, err
		}
		currentClient = cl1
	}

	byteAnswer := createAnswer(answer, encrypted)

	// send the data in the background to the right queue
	_, err := currentClient.DoBg(answer.resultQueue, byteAnswer, runtime.JobNormal)
	if err != nil {
		return currentClient, fmt.Errorf("client: %s", err.Error())
	}

	return currentClient, nil
}

func sendWorkerJobBg(args *checkGmArgs) (string, error) {
	cl1, err := buildClient(args.Host)
	if err != nil {
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

func sendWorkerJob(args *checkGmArgs) (string, error) {
	cl1, err := buildClient(args.Host)
	if err != nil {
		return "", err
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
