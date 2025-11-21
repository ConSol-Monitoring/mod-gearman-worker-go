package modgearman

import (
	"flag"
	"fmt"
	"maps"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/consol-monitoring/mod-gearman-worker-go/pkg/utils"
	"github.com/nsf/termbox-go"
)

type gmTopArgs struct {
	Usage    bool
	Verbose  bool
	Version  bool
	Host     string
	Quiet    bool
	Interval float64
	Batch    bool
	Hosts    []string
}

type dataRow struct {
	queueName       string
	workerAvailable string
	jobsWaiting     string
	jobsRunning     string
}

const (
	connTimeout = 10
)

func GearmanTop(build string) {
	args := &gmTopArgs{}
	// Define a new FlagSet for avoiding collisions with other flags
	flagSet := flag.NewFlagSet("gearman_top", flag.ExitOnError)

	flagSet.BoolVar(&args.Usage, "h", false, "Print usage")
	flagSet.BoolVar(&args.Version, "V", false, "Print version")
	flagSet.BoolVar(&args.Quiet, "q", false, "Quiet mode")
	flagSet.BoolVar(&args.Batch, "b", false, "Batch mode")
	flagSet.BoolVar(&args.Verbose, "v", false, "Verbose output")
	flagSet.Float64Var(&args.Interval, "i", 1.0, "Set interval")
	flagSet.Func("H", "Add host", func(host string) error {
		return add2HostList(host, &args.Hosts)
	})

	// Parse the flags in the custom FlagSet
	err := flagSet.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags -> %s", err.Error())
		os.Exit(1)
	}

	implementLogger()

	if args.Usage {
		printTopUsage()

		return
	}
	if args.Version {
		printTopVersion(build)

		return
	}

	hostList := createHostList(args.Hosts)

	// Map with active connections to the hosts in order to maintain a connection
	// instead of creating a new connection on every iteration
	connectionMap := make(map[string]net.Conn)

	if args.Batch {
		printInBatchMode(hostList, connectionMap)

		return
	}

	initializeTermbox()
	defer termbox.Close()

	// Print stats for host in a loop
	runInteractiveMode(args, hostList, connectionMap)
}

func createHostList(hostList []string) []string {
	if len(hostList) == 0 {
		hostList = append(hostList, "localhost")
	} else {
		hostList = unique(hostList)
	}

	return hostList
}

func runInteractiveMode(args *gmTopArgs, hostList []string, connectionMap map[string]net.Conn) {
	eventQueue := make(chan termbox.Event)
	go func() {
		for {
			eventQueue <- termbox.PollEvent()
		}
	}()

	ticker := time.NewTicker(time.Duration(args.Interval * float64(time.Second)))
	defer ticker.Stop()

	tableChan := make(chan map[string]string)
	printMap := make(map[string]string)
	var mutex sync.Mutex

	// Initialize printMap with placeholders
	for _, host := range hostList {
		printMap[host] = fmt.Sprintf("---- %s ----\nNot data yet...\n\n\n", host)
	}

	// Get and print stats for all hosts in parallel in order to prevent a program block
	// when a connection to a host runs into a timeout
	printHostsInParallel(hostList, connectionMap, tableChan, args.Interval)

	// Print once before the ticker ticks for the first time
	initPrint(&mutex, printMap, hostList, tableChan)

	for {
		select {
		case ev := <-eventQueue:
			if ev.Type == termbox.EventKey && (ev.Key == termbox.KeyEsc || ev.Ch == 'q' || ev.Ch == 'Q' || ev.Key == termbox.KeyCtrlC) {
				// Close all active connections
				for key := range connectionMap {
					if connectionMap[key] != nil {
						err := connectionMap[key].Close()
						if err != nil {
							fmt.Fprintf(os.Stdout, "Error closing connection %v\n", err)
						}
					}
				}

				return
			}
		case <-ticker.C:
			printHosts(&mutex, hostList, printMap)
		/* If a new stat is available all stats are transferred into the printMap
		which maintains the order right order of the called hosts and assigns the
		correct string (table) that should be printed */
		case tables := <-tableChan:
			mutex.Lock()
			maps.Copy(printMap, tables)
			mutex.Unlock()
		}
	}
}

func initializeTermbox() {
	err := termbox.Init()
	if err != nil {
		panic(err)
	}
}

func implementLogger() {
	cfg := &config{}
	cfg.setDefaultValues()
	createLogger(cfg)
}

func printInBatchMode(hostList []string, connectionMap map[string]net.Conn) {
	currTime := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(os.Stdout, "%s\n\n", currTime)
	for _, host := range hostList {
		fmt.Fprintln(os.Stdout, generateQueueTable(host, connectionMap))
	}
}

func initPrint(mutex *sync.Mutex, printMap map[string]string, hostList []string, tableChan chan map[string]string) {
	printHosts(mutex, hostList, printMap)
	tables := <-tableChan
	mutex.Lock()
	maps.Copy(printMap, tables)
	mutex.Unlock()
	printHosts(mutex, hostList, printMap)
}

func printHostsInParallel(hostList []string, connectionMap map[string]net.Conn, tableChan chan map[string]string, interval float64) {
	for _, host := range hostList {
		go func(host string) {
			for {
				table := generateQueueTable(host, connectionMap)
				tableChan <- map[string]string{host: table}
				time.Sleep(time.Duration(interval * float64(time.Second)))
			}
		}(host)
	}
}

func printHosts(mutex *sync.Mutex, hostList []string, printMap map[string]string) {
	mutex.Lock()
	defer mutex.Unlock()
	// Clear screen
	fmt.Fprintf(os.Stdout, "\033[H\033[2J")
	currTime := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(os.Stdout, "%s\n\n", currTime)

	for _, host := range hostList {
		if table, ok := printMap[host]; ok {
			fmt.Fprintln(os.Stdout, table)
		} else {
			fmt.Fprintf(os.Stdout, "---- %s ----", host)
			fmt.Fprintf(os.Stdout, "No data yet...\n\n")
		}
	}
}

func generateQueueTable(ogHostname string, connectionMap map[string]net.Conn) string {
	hostName := extractHostName(ogHostname)
	port, err := determinePort(ogHostname)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %s\n", err, ogHostname)
		os.Exit(1)
	}
	newAddress := fmt.Sprintf("%s:%d", hostName, port)

	queueList, version, err := processGearmanQueues(newAddress, connectionMap)
	if err != nil {
		return fmt.Sprintf("---- %s:%d ----\n%s\n\n", hostName, port, err)
	}
	if len(queueList) == 0 {
		return fmt.Sprintf("---- %s:%d ----\nNo queues have been found at host %s\n\n", hostName, port, hostName)
	}

	table, err := createTable(queueList)
	if err != nil {
		return fmt.Sprintf("---- %s:%d ----\nError: %s\n\n", hostName, port, err)
	}

	return fmt.Sprintf("---- %s:%d ----- %s\n%s", hostName, port, version, table)
}

func createTable(queueList []queue) (string, error) {
	tableHeaders := createTableHeaders()
	rows := createTableRows(queueList)
	table, err := utils.ASCIITable(tableHeaders, rows, true)
	if err != nil {
		return "", fmt.Errorf("error creating table -> %w", err)
	}

	tableSize := calcTableSize(tableHeaders)
	tableHorizontalBorder := strings.Repeat("-", tableSize+1) // Add one for an additional pipe symbol at the end of each row
	table = fmt.Sprintf("%s\n%s%s\n\n", tableHorizontalBorder, table, tableHorizontalBorder)

	return table, nil
}

func calcTableSize(tableHeaders []utils.ASCIITableHeader) int {
	tableSize := 0
	for _, header := range tableHeaders {
		tableSize += header.Size
		tableSize += 3
	}

	return tableSize
}

func createTableHeaders() []utils.ASCIITableHeader {
	tableHeaders := []utils.ASCIITableHeader{
		{
			Name:  "Queue Name",
			Field: "queueName",
		},
		{
			Name:      "Worker Available",
			Field:     "workerAvailable",
			Alignment: "right",
		},
		{
			Name:      "Jobs Waiting",
			Field:     "jobsWaiting",
			Alignment: "right",
		},
		{
			Name:      "Jobs running",
			Field:     "jobsRunning",
			Alignment: "right",
		},
	}

	return tableHeaders
}

func createTableRows(queueList []queue) []dataRow {
	rows := make([]dataRow, len(queueList))
	for i, queue := range queueList {
		rows[i] = dataRow{
			queueName:       queue.Name,
			workerAvailable: strconv.Itoa(queue.AvailWorker),
			jobsWaiting:     strconv.Itoa(queue.Waiting),
			jobsRunning:     strconv.Itoa(queue.Running),
		}
	}

	return rows
}

func printTopUsage() {
	fmt.Fprintln(os.Stdout, "usage:")
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "gearman_top   [ -H <hostname>[:port]           ]")
	fmt.Fprintln(os.Stdout, "              [ -i <sec>       seconds         ]")
	fmt.Fprintln(os.Stdout, "              [ -q             quiet mode      ]")
	fmt.Fprintln(os.Stdout, "              [ -b             batch mode      ]")
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "              [ -h             print help      ]")
	fmt.Fprintln(os.Stdout, "              [ -v             verbose output  ]")
	fmt.Fprintln(os.Stdout, "              [ -V             print version   ]")
	fmt.Fprintln(os.Stdout)

	os.Exit(0)
}

func printTopVersion(build string) {
	config := &config{binary: "check_gearman", build: build}
	printVersion(config)
	os.Exit(3)
}

func add2HostList(host string, hostList *[]string) error {
	*hostList = append(*hostList, host)

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
