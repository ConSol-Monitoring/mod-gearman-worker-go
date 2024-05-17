package modgearman

import (
	"net"
	"strconv"
	"strings"
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
		log.Errorf("%s", err)
		return []queue{}, err
	}

	if gearmanStatus == "" {
		return queueList, nil
	}

	// Organize queues into a list
	lines := strings.Split(gearmanStatus, "\n")
	for _, line := range lines {
		parts := strings.Fields(line)

		if len(parts) < 4 || (parts[0] == "dummy" && parts[1] == "") {
			continue
		}
		totalInt, err := strconv.Atoi(parts[1])
		if err != nil {
			log.Errorf("The recieved data is not in the right format: %s", err)
			return []queue{}, err
		}
		runningInt, err := strconv.Atoi(parts[2])
		if err != nil {
			log.Errorf("The recieved data is not in the right format: %s", err)
			return []queue{}, err
		}
		availWorkerInt, err := strconv.Atoi(parts[3])
		if err != nil {
			log.Errorf("The recieved data is not in the right format: %s", err)
			return []queue{}, err
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
	conn, connErr := gm_net_connect(hostname, port)
	if connErr != nil {
		return "", connErr
	}

	_, writeErr := conn.Write([]byte(cmd))
	if writeErr != nil {
		return "", writeErr
	}

	// Read response
	buffer := make([]byte, 512)
	n, readErr := conn.Read(buffer)
	if readErr != nil {
		return "", readErr
	} else {
		result := string(buffer[:n])
		return result, nil
	}
}

func gm_net_connect(hostname string, port int) (net.Conn, error) {
	addr := hostname + ":" + strconv.Itoa(port)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
