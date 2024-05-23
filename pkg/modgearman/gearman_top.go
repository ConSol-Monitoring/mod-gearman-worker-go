package modgearman

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
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

const CONNTIMEOUT = 10

var hostList = []string{}

func GearmanTop(args *Args) {
	config := &config{}
	config.setDefaultValues()
	createLogger(config)

	if args.Usage {
		printTopUsage()
		return
	}
	if args.Version {
		printTopVersion()
		return
	}

	if len(hostList) == 0 {
		hostList = append(hostList, "localhost")
	}
	hostList = unique(hostList)

	if args.Batch {
		for _, host := range hostList {
			currTime := time.Now().Format("2006-01-02 15:04:05")
			fmt.Printf("%s  -  v%s\n\n", currTime, GM_TOP_VERSION)
			fmt.Println(printStats(host))
		}
		return
	}

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

	statsChan := make(chan map[string]string)
	printMap := make(map[string]string)
	var mu sync.Mutex

	for _, host := range hostList {
		go func(host string) {
			for {
				stats := printStats(host)
				statsChan <- map[string]string{host: stats}
				time.Sleep(time.Duration(args.Interval) * time.Second)
			}
		}(host)
	}

	for {
		select {
		case ev := <-eventQueue:
			if ev.Type == termbox.EventKey && (ev.Key == termbox.KeyEsc || ev.Ch == 'q' || ev.Ch == 'Q') {
				return
			}
		case <-tick:
			mu.Lock()
			fmt.Printf("\033[H\033[2J")
			currTime := time.Now().Format("2006-01-02 15:04:05")
			fmt.Printf("%s  -  v%s\n\n", currTime, GM_TOP_VERSION)

			for _, host := range hostList {
				if stat, ok := printMap[host]; ok {
					fmt.Println(stat)
				} else {
					fmt.Println(host)
					fmt.Printf("No data yet...\n\n\n")
				}
			}
			mu.Unlock()
		case stats := <-statsChan:
			mu.Lock()
			for host, stat := range stats {
				printMap[host] = stat
			}
			mu.Unlock()
		}
	}
}

func printStats(ogHostname string) string {
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

	// Retrieve data from gearman admin and save queue data to queueList. Implement a timeout
	// queueChan := make(chan []queue, 1)
	// errChan := make(chan error, 1)
	// go func() {
	// 	queueList, err := getGearmanServerData(hostname, port)
	// 	if err != nil {
	// 		errChan <- err
	// 	} else {
	// 		queueChan <- queueList
	// 	}
	// }()

	// var queueList []queue
	// var err error
	// select {
	// case queueList = <-queueChan:
	// case err = <-errChan:
	// case <-time.After(time.Duration(CONNTIMEOUT) * time.Second):
	// 	//log.Errorf("Time out while fetching data from host %s\n", hostname)
	// 	return fmt.Sprintf("%s:%d\nTimeout while fetching data from host %s\n\n", hostname, port, hostname)
	// }

	queueList, err := getGearmanServerData(hostname, port)

	// Proccess possible errors
	if err != nil {
		return fmt.Sprintf("%s:%d\n%s\n\n", hostname, port, err)
	}
	if queueList == nil {
		return fmt.Sprintf("%s:%d\nNo queues have been found at host %s\n\n", hostname, port, hostname)
	}

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
		//fmt.Println("Error: ", err)
		return fmt.Sprintf("%s:%d\nError: %s\n\n", hostname, port, err)
	}
	return fmt.Sprintf("%s:%d\n%s", hostname, port, table)
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
