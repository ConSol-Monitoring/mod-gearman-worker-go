package modgearman

import (
	"fmt"
	"strconv"

	"github.com/consol-monitoring/snclient/pkg/utils"
)

const hostname string = "localhost"
const port int = 4730

var totalQueues []Queue

func PrintSatus() {
	// Retrieve data from gearman admin and save queue data to totalQueues
	GetGearmanServerData(hostname, port, &totalQueues)

	// Create table headers
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
