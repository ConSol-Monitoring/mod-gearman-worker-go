package modgearman

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
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

const (
	gmDefaultPort = 4730

	readBufferSize = 4000
	columnLength   = 4
)

// Logic for sorting queues alphabetically
type byQueueName []queue

func (a byQueueName) Len() int           { return len(a) }
func (a byQueueName) Less(i, j int) bool { return a[i].Name < a[j].Name }
func (a byQueueName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

func determinePort(address string) (int, error) {
	addressParts := strings.Split(address, ":")
	hostName := addressParts[0]

	switch len(addressParts) {
	case 1:
		return getDefaultPort(hostName)
	case 2:
		port, err := strconv.Atoi(addressParts[1])
		if err != nil {
			return -1, fmt.Errorf("error converting port %s to int -> %w", address, err)
		}

		return port, nil
	default:
		return -1, errors.New("too many colons in address")
	}
}

func getDefaultPort(hostname string) (int, error) {
	if hostname == "localhost" || hostname == "127.0.0.1" {
		envServer := os.Getenv("CONFIG_GEARMAND_PORT")
		if envServer != "" {
			port, err := strconv.Atoi(strings.Split(envServer, ":")[1])
			if err != nil {
				return -1, fmt.Errorf("error converting port %s to int -> %w", envServer, err)
			}

			return port, nil
		}
	}

	return gmDefaultPort, nil
}

func extractHostName(address string) string {
	return strings.Split(address, ":")[0]
}

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
	for _, row := range lines {
		columns := strings.Fields(row)

		if len(columns) == 2 && columns[0] == "OK" {
			version = columns[1]

			continue
		}

		if len(columns) < columnLength {
			continue
		}

		totalInt, err := strconv.Atoi(columns[1])
		if err != nil {
			return nil, "", fmt.Errorf("the received data is not in the right format -> %w", err)
		}
		runningInt, err := strconv.Atoi(columns[2])
		if err != nil {
			return nil, "", fmt.Errorf("the received data is not in the right format -> %w", err)
		}
		availWorkerInt, err := strconv.Atoi(columns[3])
		if err != nil {
			return nil, "", fmt.Errorf("the received data is not in the right format -> %w", err)
		}

		// Skip dummy queue if empty
		if columns[0] == "dummy" && totalInt == 0 {
			continue
		}

		queueList = append(queueList, queue{
			Name:        columns[0],
			Total:       totalInt,
			Running:     runningInt,
			AvailWorker: availWorkerInt,
			Waiting:     totalInt - runningInt,
		})
	}

	sort.Sort(byQueueName(queueList))

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
