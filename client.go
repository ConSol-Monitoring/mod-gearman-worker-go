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
func sendAnswer(answer *answer, key []byte, server string, encrypted bool) bool {
	c, err := client.New("tcp4", server)
	if err != nil {
		logger.Errorf("client: sendanswer: %s", err.Error())
		return false
	}
	defer c.Close()
	logger.Trace(answer)

	byteAnswer := createAnswer(answer, key, encrypted)

	//send the data in the background to the right queue
	c.DoBg(answer.resultQueue, byteAnswer, runtime.JobNormal)

	return true
}
