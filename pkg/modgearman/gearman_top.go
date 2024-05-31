package modgearman

import (
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
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
	Hosts    []string
}

type dataRow struct {
	queueName       string
	workerAvailable string
	jobsWaiting     string
	jobsRunning     string
}

type byQueueName []dataRow

// Logic for sorting queue names
func (a byQueueName) Len() int           { return len(a) }
func (a byQueueName) Less(i, j int) bool { return a[i].queueName < a[j].queueName }
func (a byQueueName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

const GM_TOP_VERSION = "1.1.2"
const GM_DEFAULT_PORT = 4730
const CONNTIMEOUT = 10

func GearmanTop(args *Args) {
	implementLogger()

	if args.Usage {
		printTopUsage()
		return
	}
	if args.Version {
		printTopVersion()
		return
	}

	hostList := createHostList(args.Hosts)

	// Map with active connections to the hosts in order to maintain a connection
	// instead of creating a new connection on every iteration
	connectionMap := createConnectionMap(hostList)

	if args.Batch {
		printInBatchMode(hostList, connectionMap)
	}

	initializeTermbox()
	defer termbox.Close()

	// Print stats for host in a loop
	runInteractiveMode(args, hostList, connectionMap)
}

func createConnectionMap(hostList []string) map[string]net.Conn {
	connectionMap := make(map[string]net.Conn)
	for _, host := range hostList {
		connectionMap[host] = nil
	}
	return connectionMap
}

func createHostList(hostList []string) []string {
	if len(hostList) == 0 {
		hostList = append(hostList, "localhost")
	} else {
		hostList = unique(hostList)
	}
	return hostList
}

func runInteractiveMode(args *Args, hostList []string, connectionMap map[string]net.Conn) {
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
	var mu sync.Mutex

	// Initialize printMap with placeholders
	for _, host := range hostList {
		printMap[host] = fmt.Sprintf("---- %s ----\nNot data yet...\n\n\n", host)
	}

	// Execute getStats() for all hosts in parallel in order to prevent a program block
	// when a connection to a host runs into a timeout
	for _, host := range hostList {
		go func(host string) {
			for {
				table := generateQueueTable(host, connectionMap)
				tableChan <- map[string]string{host: table}
				time.Sleep(time.Duration(args.Interval) * time.Second)
			}
		}(host)
	}

	// Print once before the ticker ticks for the first time
	initPrint(&mu, printMap, hostList, tableChan)

	for {
		select {
		case ev := <-eventQueue:
			if ev.Type == termbox.EventKey && (ev.Key == termbox.KeyEsc || ev.Ch == 'q' || ev.Ch == 'Q') {
				// Close all active connections
				for key := range connectionMap {
					if connectionMap[key] != nil {
						connectionMap[key].Close()
					}
				}
				return
			}
		case <-ticker.C:
			printHosts(&mu, hostList, printMap)
		// If a new stat is available all stats are transferred into the printMap
		// printMap maintains the order right order of the called hosts and assigns the
		// correct string (table) that should be printed
		case tables := <-tableChan:
			mu.Lock()
			for host, table := range tables {
				printMap[host] = table
			}
			mu.Unlock()
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
	for _, host := range hostList {
		currTime := time.Now().Format("2006-01-02 15:04:05")
		fmt.Printf("%s  -  v%s\n\n", currTime, GM_TOP_VERSION)
		fmt.Println(generateQueueTable(host, connectionMap))
	}
}

func initPrint(mu *sync.Mutex, printMap map[string]string, hostList []string, tableChan chan map[string]string) {
	printHosts(mu, hostList, printMap)
	tables := <-tableChan
	mu.Lock()
	for host, table := range tables {
		printMap[host] = table
	}
	mu.Unlock()
	printHosts(mu, hostList, printMap)
}

func printHosts(mu *sync.Mutex, hostList []string, printMap map[string]string) {
	mu.Lock()
	defer mu.Unlock()
	// Clear screen
	fmt.Printf("\033[H\033[2J")
	currTime := time.Now().Format("2006-01-02 15:04:05")
	fmt.Printf("%s  -  v%s\n\n", currTime, GM_TOP_VERSION)

	for _, host := range hostList {
		if table, ok := printMap[host]; ok {
			fmt.Println(table)
		} else {
			fmt.Printf("---- %s ----", host)
			fmt.Printf("No data yet...\n\n\n")
		}
	}
}

func generateQueueTable(ogHostname string, connectionMap map[string]net.Conn) string {
	hostName := extractHostName(ogHostname)
	port, err := determinePort(ogHostname)
	if err != nil {
		fmt.Printf("%s %s\n", err, ogHostname)
		os.Exit(1)
	}
	newAddress := fmt.Sprintf("%s:%d", hostName, port)

	queueList, err := processGearmanQueues(newAddress, connectionMap)
	if err != nil {
		return fmt.Sprintf("---- %s:%d ----\n%s\n\n", hostName, port, err)
	}
	if queueList == nil {
		return fmt.Sprintf("---- %s:%d ----\nNo queues have been found at host %s\n\n", hostName, port, hostName)
	}

	table, err := createTable(queueList)
	if err != nil {
		return fmt.Sprintf("---- %s:%d ----\nError: %s\n\n", hostName, port, err)
	}
	return fmt.Sprintf("---- %s:%d -----\n%s", hostName, port, table)
}

func determinePort(address string) (int, error) {
	addressParts := strings.Split(address, ":")
	hostName := addressParts[0]

	switch len(addressParts) {
	case 1:
		return getDefaultPort(hostName)
	case 2:
		return strconv.Atoi(addressParts[1])
	default:
		return -1, errors.New("too many colons in address")
	}
}

func getDefaultPort(hostname string) (int, error) {
	if hostname == "localhost" || hostname == "127.0.0.1" {
		envServer := os.Getenv("CONFIG_GEARMAND_PORT")
		if envServer != "" {
			return strconv.Atoi(strings.Split(envServer, ":")[1])
		}
	}
	return GM_DEFAULT_PORT, nil
}

func extractHostName(address string) string {
	return strings.Split(address, ":")[0]
}

func createTable(queueList []queue) (string, error) {
	tableHeaders := createTableHeaders()
	rows := createTableRows(queueList)
	table, err := utils.ASCIITable(tableHeaders, rows, true)
	if err != nil {
		return "", err
	}
	return table, nil
}

func createTableHeaders() []utils.ASCIITableHeader {
	tableHeaders := []utils.ASCIITableHeader{
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
	return tableHeaders
}

func createTableRows(queueList []queue) []dataRow {
	var rows []dataRow
	for _, queue := range queueList {

		rows = append(rows, dataRow{
			queueName:       queue.Name,
			workerAvailable: strconv.Itoa(queue.AvailWorker),
			jobsWaiting:     strconv.Itoa(queue.Waiting),
			jobsRunning:     strconv.Itoa(queue.Running),
		})
	}
	sort.Sort(byQueueName(rows))
	return rows
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

func Add2HostList(host string, hostList *[]string) error {
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
