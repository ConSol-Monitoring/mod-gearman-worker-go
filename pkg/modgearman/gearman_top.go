package modgearman

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/consol-monitoring/snclient/pkg/utils"
	"github.com/nsf/termbox-go"
)

type Args struct {
	Usage    bool
	Verbose  bool
	Version  bool
	Host     string
	Quiet    bool
	Interval float64
	Batch    bool
}

const GM_TOP_VERSION = "1.1.2"
const GM_DEFAULT_PORT = 4730

var hostList = []string{}

func GearmanTop(args *Args) {
	if args.Usage {
		printTopUsage()
	}
	if args.Version {
		printTopVersion()
	}

	if len(hostList) == 0 {
		hostList = append(hostList, "localhost")
	}
	hostList = unique(hostList)

	// Print stats only once when using batch mode
	if args.Batch {
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

	tick := time.Tick(time.Duration(args.Interval * float64(time.Second)))

	for {
		select {
		case ev := <-eventQueue:
			if ev.Type == termbox.EventKey && (ev.Key == termbox.KeyEsc || ev.Ch == 'q' || ev.Ch == 'Q') {
				return // Exit if 'q' is pressed
			}
		case <-tick:
			// Clear screen
			fmt.Printf("\033[H\033[2J")
			for _, host := range hostList {
				printStats(host)
			}
		}
	}
}

func printStats(ogHostname string) {
	var port int

	// Determine port of hostname
	hostAddress := strings.Split(ogHostname, ":")
	hostname := hostAddress[0]
	if len(hostAddress) > 2 {
		err := errors.New("too many colons in host address")
		fmt.Printf("%s %s\n", err, ogHostname)
		os.Exit(1)
	} else if len(hostAddress) == 2 {
		port, _ = strconv.Atoi(hostAddress[1])
	} else {
		// Get port from gearman config if program is started on the same environment
		envServer := os.Getenv("CONFIG_GEARMAND_PORT")
		if envServer != "" {
			port, _ = strconv.Atoi(strings.Split(envServer, ":")[1])
		} else {
			port = GM_DEFAULT_PORT
		}
	}

	// Retrieve data from gearman admin and save queue data to queueList
	queueList := getGearmanServerData(hostname, port)

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
	currTime := time.Now().Format("2006-01-02 15:04:05")
	fmt.Printf("%s  -  %s:%d  -  v%s\n\n", currTime, hostname, port, GM_TOP_VERSION)
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
	fmt.Printf("gearman_top: version %s\n", GM_TOP_VERSION)
	os.Exit(0)
}

func Add2HostList(host string) error {
	hostList = append(hostList, host)
	return nil
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
