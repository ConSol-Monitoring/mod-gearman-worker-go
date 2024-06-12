package modgearman

import (
	"bytes"
	"errors"
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

const readBufferSize = 4000

func processGearmanQueues(address string, connectionMap map[string]net.Conn) ([]queue, string, error) {
	payload, err := queryGermanInstance(address, connectionMap)
	if err != nil {
		return nil, "", err
	}
	// Split retrieved payload and extract and store data
	version := ""
	lines := strings.Split(payload, "\n")

	if len(lines) == 0 {
		return nil, "", nil
	}

	queueList := []queue{}
	for _, line := range lines {
		parts := strings.Fields(line)

		if len(parts) == 2 && parts[0] == "OK" {
			version = parts[1]
			continue
		}

		if len(parts) < 4 || (parts[0] == "dummy" && parts[1] == "") {
			continue
		}

		totalInt, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, "", fmt.Errorf("the received data is not in the right format -> %w", err)
		}
		runningInt, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil, "", fmt.Errorf("the received data is not in the right format -> %w", err)
		}
		availWorkerInt, err := strconv.Atoi(parts[3])
		if err != nil {
			return nil, "", fmt.Errorf("the received data is not in the right format -> %w", err)
		}

		queueList = append(queueList, queue{
			Name:        parts[0],
			Total:       totalInt,
			Running:     runningInt,
			AvailWorker: availWorkerInt,
			Waiting:     totalInt - runningInt,
		})
	}

	// Add v before version number for better formatting
	if version != "" {
		version = fmt.Sprintf("v%s", version)
	}

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
			return "", err
		}
		connectionMap[address] = conn
	}
	if err := writeConnection(conn, "status\nversion\n"); err != nil {
		delete(connectionMap, address)

		return "", err
	}

	payload, err := readConnection(conn)
	if err != nil {
		return "", err
	}

	return payload, nil
}

func makeConnection(address string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", address, connTimeout*time.Second)
	if err != nil {
		return nil, fmt.Errorf("timeout or error with tcp connection -> %w", err)
	}

	return conn, nil
}

func writeConnection(conn net.Conn, cmd string) error {
	err := conn.SetWriteDeadline(time.Now().Add(connTimeout * time.Second))
	if err != nil {
		return fmt.Errorf("error while setting write deadline -> %w", err)
	}
	_, err = conn.Write([]byte(cmd))
	if err != nil {
		return fmt.Errorf("error while writing to tcp connection -> %w", err)
	}

	return nil
}

func readConnection(conn net.Conn) (string, error) {
	err := conn.SetReadDeadline(time.Now().Add(connTimeout * time.Second))
	if err != nil {
		return "", fmt.Errorf("error while setting read deadline -> %w", err)
	}
	var buffer bytes.Buffer
	tmp := make([]byte, readBufferSize)

	for {
		numReadBytes, err := conn.Read(tmp)
		if numReadBytes > 0 {
			buffer.Write(tmp[:numReadBytes])
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return "", fmt.Errorf("error while reading from tcp connection -> %w", err)
		}
		if numReadBytes > 0 && tmp[numReadBytes-1] == '\n' {
			break
		}
	}

	return buffer.String(), nil
}
