package modgearman

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

type queue struct {
	Name        string // queue names
	Total       int    // total number of jobs
	Running     int    // number of running jobs
	Waiting     int    // number of waiting jobs
	AvailWorker int    // total number of available worker
}

func getGearmanServerData(hostname string, port int) ([]queue, error) {
	var queueList []queue
	gearmanStatus, err := sendCmd2gearmandAdmin("status\nversion\n", hostname, port)

	if err != nil {
		return nil, err
	}

	if gearmanStatus == "" {
		return queueList, nil
	}

	// Split recieved answer-string and set the data in relation
	lines := strings.Split(gearmanStatus, "\n")
	for _, line := range lines {
		parts := strings.Fields(line)

		if len(parts) < 4 || (parts[0] == "dummy" && parts[1] == "") {
			continue
		}
		totalInt, err := strconv.Atoi(parts[1])
		if err != nil {
			err := fmt.Errorf("the recieved data is not in the right format: %s", err)
			return nil, err
		}
		runningInt, err := strconv.Atoi(parts[2])
		if err != nil {
			err := fmt.Errorf("the recieved data is not in the right format: %s", err)
			return nil, err
		}
		availWorkerInt, err := strconv.Atoi(parts[3])
		if err != nil {
			err := fmt.Errorf("the recieved data is not in the right format: %s", err)
			return nil, err
		}

		queueList = append(queueList, queue{
			Name:        parts[0],
			Total:       totalInt,
			Running:     runningInt,
			AvailWorker: availWorkerInt,
			Waiting:     totalInt - runningInt,
		})
	}

	return queueList, nil
}

func sendCmd2gearmandAdmin(cmd string, hostname string, port int) (string, error) {
	addr := hostname + ":" + strconv.Itoa(port)

	// Look for existing connection in connMap
	// If no connection is found establish a new one with the host and save it to connMap for future use
	conn := connMap[addr]
	if conn == nil {
		var err error
		conn, err = net.DialTimeout("tcp", addr, CONNTIMEOUT*time.Second)
		if err != nil {
			return "", err
		}
		connMap[addr] = conn
	}

	// Set and reset timeout for established connection
	conn.SetDeadline(time.Now().Add(CONNTIMEOUT * time.Second))

	_, writeErr := conn.Write([]byte(cmd))
	if writeErr != nil {
		connMap[addr] = nil
		return "", writeErr
	}

	// Read response
	var buffer bytes.Buffer
	tmp := make([]byte, 4000)

	for {
		n, readErr := conn.Read(tmp)
		if n > 0 {
			buffer.Write(tmp[:n])
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			connMap[addr] = nil
			return "", readErr
		}
		if n > 0 && tmp[n-1] == '\n' {
			break
		}
	}
	return buffer.String(), nil
}
