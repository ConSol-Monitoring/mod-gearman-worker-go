package modgearman

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/consol-monitoring/snclient/pkg/utils"
)

type queue struct {
	Name        string // queue names
	Total       int    // total number of jobs
	Running     int    // number of running jobs
	Waiting     int    // number of waiting jobs
	AvailWorker int    // total number of available worker
}

var totalQueues []queue

func PrintSatus() {
	//Create table headers
	var tableHeaders = []utils.ASCIITableHeader{
		{
			Name:     "Queue Name",
			Field:    "queueName",
			Centered: false,
		},
		{
			Name:     "Worker Available",
			Field:    "workerAvailable",
			Centered: false,
		},
		{
			Name:     "Jobs Waiting",
			Field:    "jobsWaiting",
			Centered: false,
		},
		{
			Name:     "Jobs running",
			Field:    "jobsRunning",
			Centered: false,
		},
	}

	// Create table rows
	type DataRow struct {
		queueName       string
		workerAvailable string
		jobsWaiting     string
		jobsRunning     string
	}

	var rows []DataRow
	for _, queue := range totalQueues {

		rows = append(rows, DataRow{
			queueName:       queue.Name,
			workerAvailable: strconv.Itoa(queue.AvailWorker),
			jobsWaiting:     strconv.Itoa(queue.Waiting),
			jobsRunning:     strconv.Itoa(queue.Running),
		})
	}

	table, err := utils.ASCIITable(tableHeaders, rows, true)
	if err != nil {
		fmt.Println("Error: ", err)
		return
	}
	fmt.Println(table)
}

func GetGearmanServerData(hostname string, port int) {
	var gearmanStatus string = Send2gearmandadmin("status\nversion\n", hostname, port)

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

		totalQueues = append(totalQueues, queue{
			Name:        parts[0],
			Total:       totalInt,
			Running:     runningInt,
			AvailWorker: availWorkerInt,
			Waiting:     totalInt - runningInt,
		})
	}

	PrintSatus()
}

func Send2gearmandadmin(cmd string, hostname string, port int) string {
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
