package modgearman

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

type Queue struct {
	Name        string // queue names
	Total       int    // total number of jobs
	Running     int    // number of running jobs
	Waiting     int    // number of waiting jobs
	AvailWorker int    // total number of available worker
}

func GetGearmanServerData(hostname string, port int, queueList *[]Queue) {
	var gearmanStatus string = SendCmd2gearmandAdmin("status\nversion\n", hostname, port)

	if gearmanStatus == "" {
		return
	}

	// Organize queues into a list
	lines := strings.Split(gearmanStatus, "\n")
	for _, line := range lines {
		parts := strings.Fields(line)

		if len(parts) < 4 || parts[0] == "dummy" {
			continue
		}
		totalInt, _ := strconv.Atoi(parts[1])
		runningInt, _ := strconv.Atoi(parts[2])
		availWorkerInt, _ := strconv.Atoi(parts[3])

		*queueList = append(*queueList, Queue{
			Name:        parts[0],
			Total:       totalInt,
			Running:     runningInt,
			AvailWorker: availWorkerInt,
			Waiting:     totalInt - runningInt,
		})
	}
}

func SendCmd2gearmandAdmin(cmd string, hostname string, port int) string {
	conn, connErr := gm_net_connect(hostname, port)
	if connErr != nil {
		fmt.Println(connErr)
		return ""
	}

	_, writeErr := conn.Write([]byte(cmd))
	if writeErr != nil {
		fmt.Println(writeErr)
		return ""
	}

	// Read response
	buffer := make([]byte, 512)
	n, readErr := conn.Read(buffer)
	if readErr != nil {
		fmt.Println(readErr)
		return ""
	} else {
		result := string(buffer[:n])
		fmt.Println(result)
		return result
	}
}

func gm_net_connect(hostname string, port int) (net.Conn, error) {
	addr := hostname + ":" + strconv.Itoa(port)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	// Success
	return conn, nil
}
