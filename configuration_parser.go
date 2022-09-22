package modgearman

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

type configurationStruct struct {
	name                      string
	build                     string
	identifier                string
	debug                     int
	logfile                   string
	logmode                   string
	dupserver                 []string
	eventhandler              bool
	notifications             bool
	services                  bool
	hosts                     bool
	hostgroups                []string
	servicegroups             []string
	encryption                bool
	key                       string
	keyfile                   string
	pidfile                   string
	jobTimeout                int
	minWorker                 int
	maxWorker                 int
	idleTimeout               int
	maxAge                    int
	spawnRate                 int
	sinkRate                  int
	loadLimit1                float64
	loadLimit5                float64
	loadLimit15               float64
	memLimit                  int
	backgroundingThreshold    int
	showErrorOutput           bool
	dupResultsArePassive      bool
	dupServerBacklogQueueSize int
	gearmanConnectionTimeout  int
	restrictPath              []string
	server                    []string
	timeoutReturn             int
	daemon                    bool
	prometheusServer          string
	// send_gearman specific
	timeout     int
	delimiter   string
	host        string
	service     string
	resultQueue string
	returnCode  int
	message     string
	active      bool
	startTime   float64
	finishTime  float64
	latency     float64
	// worker debug profile
	flagProfile    string
	flagCPUProfile string
	flagMemProfile string
}

func setDefaultValues(result *configurationStruct) {
	result.logmode = "automatic"
	result.encryption = true
	result.showErrorOutput = true
	result.debug = 0
	result.logmode = "automatic"
	result.dupResultsArePassive = true
	result.dupServerBacklogQueueSize = 1000
	result.gearmanConnectionTimeout = -1
	result.timeoutReturn = 2
	result.jobTimeout = 60
	result.idleTimeout = 10
	result.daemon = false
	result.minWorker = 1
	result.maxWorker = 20
	result.spawnRate = 3
	result.sinkRate = 1
	result.backgroundingThreshold = 30
	result.memLimit = 70
	hostname, _ := os.Hostname()
	result.identifier = hostname
	if result.identifier == "" {
		result.identifier = "unknown"
	}
	result.delimiter = "\t"
}

// remove duplicate entries from all string lists
func (config *configurationStruct) removeDuplicates() {
	config.server = removeDuplicateStrings(config.server)
	config.dupserver = removeDuplicateStrings(config.dupserver)
	config.hostgroups = removeDuplicateStrings(config.hostgroups)
	config.servicegroups = removeDuplicateStrings(config.servicegroups)
	config.restrictPath = removeDuplicateStrings(config.restrictPath)
}

/**
* parses the key value pairs and stores them in the configuration struct
*
 */
func readSetting(values []string, result *configurationStruct) {
	key := strings.ToLower(strings.Trim(values[0], " "))
	value := strings.Trim(values[1], " ")

	switch key {
	case "dupserver":
		list := strings.Split(value, ",")
		result.dupserver = append(result.dupserver, list...)
	case "hostgroups":
		list := strings.Split(value, ",")
		for i := 0; i < len(list); i++ {
			list[i] = strings.Trim(list[i], " ")
		}
		result.hostgroups = append(result.hostgroups, list...)
	case "servicegroups":
		list := strings.Split(value, ",")
		for i := 0; i < len(list); i++ {
			list[i] = strings.Trim(list[i], " ")
		}
		result.servicegroups = append(result.servicegroups, list...)
	case "server":
		list := strings.Split(value, ",")
		for i := 0; i < len(list); i++ {
			list[i] = strings.Trim(list[i], " ")
		}
		for i, s := range list {
			list[i] = fixGearmandServerAddress(s)
		}
		result.server = append(result.server, list...)
	case "prometheus_server":
		result.prometheusServer = value
	case "timeout_return":
		result.timeoutReturn = getInt(value)
	case "config":
		readSettingsFile(value, result)
	case "debug":
		result.debug = getInt(value)
	case "logfile":
		result.logfile = value
	case "logmode":
		result.logmode = value
	case "identifier":
		result.identifier = value
	case "eventhandler":
		result.eventhandler = getBool(value)
	case "notifications":
		result.notifications = getBool(value)
	case "services":
		result.services = getBool(value)
	case "hosts":
		result.hosts = getBool(value)
	case "encryption":
		result.encryption = getBool(value)
	case "key":
		result.key = value
	case "keyfile":
		result.keyfile = value
	case "pidfile":
		result.pidfile = value
	case "job_timeout":
		result.jobTimeout = getInt(value)
	case "min-worker":
		result.minWorker = getInt(value)
	case "max-worker":
		result.maxWorker = getInt(value)
	case "idle-timeout":
		result.idleTimeout = getInt(value)
	case "max-age":
		result.maxAge = getInt(value)
	case "spawn-rate":
		result.spawnRate = getInt(value)
	case "sink-rate":
		result.sinkRate = getInt(value)
	case "load_limit1":
		result.loadLimit1 = getFloat(value)
	case "load_limit5":
		result.loadLimit5 = getFloat(value)
	case "load_limit15":
		result.loadLimit15 = getFloat(value)
	case "mem_limit":
		result.memLimit = getInt(value)
	case "backgrounding-threshold":
		result.backgroundingThreshold = getInt(value)
	case "show_error_output":
		result.showErrorOutput = getBool(value)
	case "dup_results_are_passive":
		result.dupResultsArePassive = getBool(value)
	case "dupserver_backlog_queue_size":
		result.dupServerBacklogQueueSize = getInt(value)
	case "gearman_connection_timeout":
		result.gearmanConnectionTimeout = getInt(value)
	case "restrict_path":
		result.restrictPath = append(result.restrictPath, value)
	case "timeout", "t":
		result.timeout = getInt(value)
	case "delimiter", "d":
		result.delimiter = value
	case "host":
		result.host = value
	case "service":
		result.service = value
	case "result_queue":
		result.resultQueue = value
	case "message", "m":
		result.message = value
	case "return_code", "r":
		result.returnCode = getInt(value)
	case "active":
		result.active = getBool(value)
	case "starttime":
		result.startTime = getFloat(value)
	case "finishtime":
		result.finishTime = getFloat(value)
	case "latency":
		result.latency = getFloat(value)
	case "debug-profiler":
		result.flagProfile = value
	case "cpuprofile":
		result.flagCPUProfile = value
	case "memprofile":
		result.flagMemProfile = value
	}
}

// opens the config file and reads all key value pairs, separated through = and commented out with #
// also reads the config files specified in the config= value
func readSettingsFile(path string, result *configurationStruct) {
	file, err := os.Open(path)

	if err != nil {
		logger.Error("config file not found")
		return
	}

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		// get line and remove whitespaces
		line := scanner.Text()
		line = strings.TrimSpace(line)
		// check if not commented out
		if len(line) > 0 && line[0] != '#' {
			// get both values
			values := strings.SplitN(line, "=", 2)
			readSetting(values, result)
		}
	}
}

func getInt(input string) int {
	if input == "" {
		return 0
	}
	result, err := strconv.Atoi(input)
	if err != nil {
		// check if it is an float value
		logger.Debugf("Error converting %s to int, try with float", input)
		result = int(getFloat(input))
	}
	return result
}

func getFloat(input string) float64 {
	if input == "" {
		return float64(0)
	}
	result, err := strconv.ParseFloat(input, 64)
	if err != nil {
		logger.Errorf("error Converting %s to float", input)
		result = 0
	}
	return result
}

func getBool(input string) bool {
	if input == "yes" || input == "on" || input == "1" {
		return true
	}
	return false
}

func fixGearmandServerAddress(address string) string {
	parts := strings.SplitN(address, ":", 2)
	// if no port is given, use default gearmand port
	if len(parts) == 1 {
		return address + ":4730"
	}
	// if no hostname is given, use all interfaces
	if len(parts) == 2 && parts[0] == "" {
		return "0.0.0.0:" + parts[1]
	}
	return address
}

func removeDuplicateStrings(elements []string) []string {
	encountered := map[string]bool{}
	uniq := []string{}

	for v := range elements {
		if !encountered[elements[v]] {
			encountered[elements[v]] = true
			uniq = append(uniq, elements[v])
		}
	}
	return uniq
}
