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
func sendAnswer(answer *answer, key []byte, server string) bool {
	c, err := client.New("tcp4", server)
	if err != nil {
		logger.Error("client: sendanswer: %s \n", err.Error())
		return false
	}
	defer c.Close()
	logger.Debug("\n", answer, "\n")

	byteAnswer := []byte(createAnswer(answer, key))
	logger.Debug("sending to server String: " + string(byteAnswer))

	echomsg, err := c.Echo(byteAnswer)
	if err != nil {
		logger.Error("client: sendAnswer: %s \n", err.Error())
		return false
	}

	//send the data in the background to the right queue
	c.DoBg(answer.resultQueue, echomsg, runtime.JobNormal)

	return true
}
