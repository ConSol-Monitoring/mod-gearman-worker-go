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

func processGearmanQueues(address string, connectionMap map[string]net.Conn) ([]queue, error) {
	payload, err := queryGermanInstance(address, connectionMap)
	if err != nil {
		return nil, err
	}
	// Split recieved payload and extract and store data
	var queueList []queue
	lines := strings.Split(payload, "\n")
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

func queryGermanInstance(address string, connectionMap map[string]net.Conn) (string, error) {
	// Look for existing connection in connMap
	// If no connection is found establish a new one with the host and save it to connMap for future use
	conn := connectionMap[address]
	if conn == nil {
		var connErr error
		conn, connErr = makeConnection(address)
		if connErr != nil {
			return "", connErr
		}
		connectionMap[address] = conn
	}
	writeErr := writeConnection(conn, "status\nversion\n")
	if writeErr != nil {
		connectionMap[address] = nil
		return "", writeErr
	}
	payload, readErr := readConnection(conn)
	if readErr != nil {
		return "", readErr
	}
	return payload, nil
}

func makeConnection(address string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", address, CONNTIMEOUT*time.Second)
	if err != nil {
		return nil, err
	}
	//conn.SetDeadline(time.Now().Add(CONNTIMEOUT * time.Second))
	return conn, nil
}

func writeConnection(conn net.Conn, cmd string) error {
	conn.SetWriteDeadline(time.Now().Add(CONNTIMEOUT * time.Second))
	_, err := conn.Write([]byte(cmd))
	if err != nil {
		return err
	}
	return nil
}

func readConnection(conn net.Conn) (string, error) {
	conn.SetReadDeadline(time.Now().Add(CONNTIMEOUT * time.Second))
	var buffer bytes.Buffer
	tmp := make([]byte, 4000)

	for {
		n, err := conn.Read(tmp)
		if n > 0 {
			buffer.Write(tmp[:n])
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
		if n > 0 && tmp[n-1] == '\n' {
			break
		}
	}

	return buffer.String(), nil
}