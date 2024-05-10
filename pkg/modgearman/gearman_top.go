package modgearman

import (
	"fmt"
	"os"
	"strconv"

	"github.com/consol-monitoring/snclient/pkg/utils"
)

type Args struct {
	H_usage    bool
	V_verbose  bool
	V_version  bool
	H_host     string
	Q_quiet    bool
	I_interval int
	B_batch    bool
}

var hostList = []string{"localhost"}

func GearmanTop(args *Args) {
	// Process args
	if args.H_usage {
		printTopUsage()
	}
	if args.V_version {
		printTopVersion()
	}

	// Print stats once when using batch mode
	if args.B_batch {
		for _, host := range hostList {
			PrintStats(host)
		}
		return
	}
	// Print stats in a loop

}

func PrintStats(hostname string) {
	var queueList []Queue
	const port int = 4730

	// Retrieve data from gearman admin and save queue data to queueList
	GetGearmanServerData(hostname, port, &queueList)

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
	for _, queue := range queueList {

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

func printTopUsage() {
	fmt.Println("usage:")
	fmt.Println()
	fmt.Println("gearman_top   [ -H <hostname>[:port]           ]")
	fmt.Println("              [ -i <sec>       seconds         ]")
	fmt.Println("              [ -q             quiet mode      ]")
	fmt.Println("              [ -b             batch mode      ]")
	fmt.Println()
	fmt.Println("              [ -h             print help      ]")
	fmt.Println("              [ -v             verbose output  ]")
	fmt.Println("              [ -V             print version   ]")
	fmt.Println()

	os.Exit(0)
}

func printTopVersion() {
	fmt.Println("gearman_top: version ", "5.1.3")
	os.Exit(0)
}

func Add2HostList(host string) error {
	hostList = append(hostList, host)
	return nil
}
