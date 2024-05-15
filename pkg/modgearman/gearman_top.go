package modgearman

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	time "time"

	"github.com/consol-monitoring/snclient/pkg/utils"
	"github.com/nsf/termbox-go"
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
	if args.H_usage {
		printTopUsage()
	}
	if args.V_version {
		printTopVersion()
	}

	hostList = unique(hostList)

	// Print stats only once when using batch mode
	if args.B_batch {
		for _, host := range hostList {
			printStats(host)
		}
		return
	}
	// Print stats in a loop
	err := termbox.Init()
	if err != nil {
		panic(err)
	}
	defer termbox.Close()

	eventQueue := make(chan termbox.Event)
	go func() {
		for {
			eventQueue <- termbox.PollEvent()
		}
	}()

	tick := time.Tick(time.Duration(args.I_interval) * time.Second)

	for {
		select {
		case ev := <-eventQueue:
			if ev.Type == termbox.EventKey && (ev.Key == termbox.KeyEsc || ev.Ch == 'q' || ev.Ch == 'Q') {
				return // Exit if 'q' is pressed
			}
		case <-tick:
			clearScreen()
			for _, host := range hostList {
				printStats(host)
			}
		}
	}
}

func printStats(hostname string) {
	var queueList []queue
	const port int = 4730

	// Retrieve data from gearman admin and save queue data to queueList
	getGearmanServerData(hostname, port, &queueList)

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

	type dataRow struct {
		queueName       string
		workerAvailable string
		jobsWaiting     string
		jobsRunning     string
	}

	var rows []dataRow
	for _, queue := range queueList {

		rows = append(rows, dataRow{
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
	// TODO: print hostname as IP-Address and version
	currTime := time.Now().Format("2006-01-02 15:04:05")
	fmt.Printf("%s  -  %s:%d\n\n", currTime, hostname, port)
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

func clearScreen() {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "cls")
	} else {
		cmd = exec.Command("clear")
	}
	cmd.Stdout = os.Stdout
	cmd.Run()
}

func unique[T comparable](input []T) []T {
	seen := make(map[T]bool)
	result := []T{}

	for _, v := range input {
		if _, exists := seen[v]; !exists {
			seen[v] = true
			result = append(result, v)
		}
	}

	return result
}
