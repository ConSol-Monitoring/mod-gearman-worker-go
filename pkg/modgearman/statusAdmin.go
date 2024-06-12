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

func processGearmanQueues(address string, connectionMap map[string]net.Conn) ([]queue, string, error) {
	payload, err := queryGermanInstance(address, connectionMap)
	if err != nil {
		return nil, "", err
	}
	// Split retrieved payload and extract and store data
	version := ""
	var queueList []queue
	lines := strings.Split(payload, "\n")
	for _, line := range lines {
		parts := strings.Fields(line)

		if len(parts) == 2 {
			version = parts[1]
			continue
		}

		if len(parts) < 4 || (parts[0] == "dummy" && parts[1] == "") {
			continue
		}
		totalInt, err := strconv.Atoi(parts[1])
		if err != nil {
			err := fmt.Errorf("the recieved data is not in the right format: %w", err)
			return nil, "", err
		}
		runningInt, err := strconv.Atoi(parts[2])
		if err != nil {
			err := fmt.Errorf("the recieved data is not in the right format: %w", err)
			return nil, "", err
		}
		availWorkerInt, err := strconv.Atoi(parts[3])
		if err != nil {
			err := fmt.Errorf("the recieved data is not in the right format: %w", err)
			return nil, "", err
		}

		queueList = append(queueList, queue{
			Name:        parts[0],
			Total:       totalInt,
			Running:     runningInt,
			AvailWorker: availWorkerInt,
			Waiting:     totalInt - runningInt,
		})
	}

	version = fmt.Sprintf("v%s", version)
	return queueList, version, nil
}

func queryGermanInstance(address string, connectionMap map[string]net.Conn) (string, error) {
	// Look for existing connection in connMap
	// If no connection is found establish a new one with the host and save it to connMap for future use
	conn, exists := connectionMap[address]
	if !exists {
		var err error
		conn, err = makeConnection(address)
		if err != nil {
			return "", fmt.Errorf("error with tcp connection -> %w", err)
		}
		connectionMap[address] = conn
	}
	if err := writeConnection(conn, "status\nversion\n"); err != nil {
		delete(connectionMap, address)
		return "", fmt.Errorf("error with tcp connection -> %w", err)
	}

	payload, err := readConnection(conn)
	if err != nil {
		return "", fmt.Errorf("error with tcp connection -> %w", err)
	}
	return payload, nil
}

func makeConnection(address string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", address, CONNTIMEOUT*time.Second)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func writeConnection(conn net.Conn, cmd string) error {
	err := conn.SetWriteDeadline(time.Now().Add(CONNTIMEOUT * time.Second))
	if err != nil {
		return err
	}
	_, err = conn.Write([]byte(cmd))
	if err != nil {
		return err
	}
	return nil
}

func readConnection(conn net.Conn) (string, error) {
	err := conn.SetReadDeadline(time.Now().Add(CONNTIMEOUT * time.Second))
	if err != nil {
		return "", err
	}
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
