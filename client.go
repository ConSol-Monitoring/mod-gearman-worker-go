package main

import (
	"github.com/appscode/g2/client"
	"github.com/appscode/g2/pkg/runtime"
)

/**
*@input answer: the struct containing the data to be sent
*@input key: the key for the aes encryption
*@input server: the server:port address where to send the data
 */
func sendAnswer(c *client.Client, answer *answer, key []byte, server string, encrypted bool) (*client.Client, error) {
	if c == nil {
		cl, err := client.New("tcp4", server)
		if err != nil {
			return nil, err
		}
		c = cl
	}

	logger.Trace(answer)

	byteAnswer := createAnswer(answer, key, encrypted)

	//send the data in the background to the right queue
	_, err := c.DoBg(answer.resultQueue, byteAnswer, runtime.JobNormal)

	return c, err
}
