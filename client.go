package modgearman

import (
	"github.com/appscode/g2/client"
	"github.com/appscode/g2/pkg/runtime"
)

/**
*@input answer: the struct containing the data to be sent
*@input server: the server:port address where to send the data
 */
func sendAnswer(c *client.Client, answer *answer, server string, encrypted bool) (*client.Client, error) {
	if c == nil {
		cl, err := client.New("tcp", server)
		if err != nil {
			return nil, err
		}
		c = cl
	}

	logger.Tracef("result\n%s", answer)

	byteAnswer := createAnswer(answer, encrypted)

	//send the data in the background to the right queue
	_, err := c.DoBg(answer.resultQueue, byteAnswer, runtime.JobNormal)

	return c, err
}
