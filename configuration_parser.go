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
	config                    []string
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
	forkOnExec                bool
	loadLimit1                float64
	loadLimit5                float64
	loadLimit15               float64
	showErrorOutput           bool
	dupResultsArePassive      bool
	enableEmbeddedPerl        bool
	useEmbeddedPerlImplicitly bool
	usePerlCache              bool
	p1File                    string
	gearmanConnectionTimeout  int
	restrictPath              []string
	workaroundRc25            bool
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
}

func setDefaultValues(result *configurationStruct) {
	result.logmode = "automatic"
	result.encryption = true
	result.showErrorOutput = true
	result.debug = 0
	result.logmode = "automatic"
	result.dupResultsArePassive = true
	result.gearmanConnectionTimeout = -1
	result.timeoutReturn = 2
	result.jobTimeout = 60
	result.idleTimeout = 10
	result.daemon = false
	result.minWorker = 1
	result.maxWorker = 20
	result.spawnRate = 1
	hostname, _ := os.Hostname()
	result.identifier = hostname
	if result.identifier == "" {
		result.identifier = "unknown"
	}
	result.delimiter = "\t"
}

/**
* parses the key value pairs and stores them in the configuration struct
*
 */
func readSetting(values []string, result *configurationStruct) {
	values[0] = strings.Trim(values[0], " ")
	values[1] = strings.Trim(values[1], " ")
	//check for the special cases

	if values[0] == "dupserver" {
		list := strings.Split(values[1], ",")
		result.dupserver = append(result.dupserver, list...)
	}
	if values[0] == "hostgroups" {
		list := strings.Split(values[1], ",")
		for i := 0; i < len(list); i++ {
			list[i] = strings.Trim(list[i], " ")
		}
		result.hostgroups = append(result.hostgroups, list...)
	}
	if values[0] == "servicegroups" {
		list := strings.Split(values[1], ",")
		for i := 0; i < len(list); i++ {
			list[i] = strings.Trim(list[i], " ")
		}
		result.servicegroups = append(result.servicegroups, list...)
	}
	if values[0] == "server" {
		list := strings.Split(values[1], ",")
		for i := 0; i < len(list); i++ {
			list[i] = strings.Trim(list[i], " ")
		}
		for i, s := range list {
			list[i] = fixGearmandServerAddress(s)
		}
		result.server = append(result.server, list...)
	}
	if values[0] == "prometheus_server" {
		result.prometheusServer = values[1]
	}
	if values[0] == "timeout_return" {
		result.timeoutReturn = getInt(values[1])
	}
	if values[0] == "config" {
		readSettingsFile(values[1], result)
	}
	if values[0] == "debug" {
		result.debug = getInt(values[1])
	}
	if values[0] == "logfile" {
		result.logfile = values[1]
	}
	if values[0] == "logmode" {
		result.logmode = values[1]
	}
	if values[0] == "identifier" {
		result.identifier = values[1]
	}
	if values[0] == "eventhandler" {
		result.eventhandler = getBool(values[1])
	}
	if values[0] == "notifications" {
		result.notifications = getBool(values[1])
	}
	if values[0] == "services" {
		result.services = getBool(values[1])
	}
	if values[0] == "hosts" {
		result.hosts = getBool(values[1])
	}
	if values[0] == "encryption" {
		result.encryption = getBool(values[1])
	}
	if values[0] == "key" {
		result.key = values[1]
	}
	if values[0] == "keyfile" {
		result.keyfile = values[1]
	}
	if values[0] == "pidfile" {
		result.pidfile = values[1]
	}
	if values[0] == "job_timeout" {
		result.jobTimeout = getInt(values[1])
	}
	if values[0] == "min-worker" {
		result.minWorker = getInt(values[1])
	}
	if values[0] == "max-worker" {
		result.maxWorker = getInt(values[1])
	}
	if values[0] == "idle-timeout" {
		result.idleTimeout = getInt(values[1])
	}
	if values[0] == "max-age" {
		result.maxAge = getInt(values[1])
	}
	if values[0] == "spawn-rate" {
		result.spawnRate = getInt(values[1])
	}
	if values[0] == "fork_on_exec" {
		result.forkOnExec = getBool(values[1])
	}
	if values[0] == "load_limit1" {
		result.loadLimit1 = getFloat(values[1])
	}
	if values[0] == "load_limit5" {
		result.loadLimit5 = getFloat(values[1])
	}
	if values[0] == "load_limit15" {
		result.loadLimit15 = getFloat(values[1])
	}
	if values[0] == "show_error_output" {
		result.showErrorOutput = getBool(values[1])
	}
	if values[0] == "dup_results_are_passive" {
		result.dupResultsArePassive = getBool(values[1])
	}
	if values[0] == "enable_embedded_perl" {
		result.enableEmbeddedPerl = getBool(values[1])
	}
	if values[0] == "use_embedded_perl_implicitly" {
		result.useEmbeddedPerlImplicitly = getBool(values[1])
	}
	if values[0] == "use_perl_cache" {
		result.usePerlCache = getBool(values[1])
	}
	if values[0] == "p1_file" {
		result.p1File = values[1]
	}
	if values[0] == "gearman_connection_timeout" {
		result.gearmanConnectionTimeout = getInt(values[1])
	}
	if values[0] == "restrict_path" {
		result.restrictPath = append(result.restrictPath, values[1])
	}
	if values[0] == "workaround_rc_25" {
		result.workaroundRc25 = getBool(values[1])
	}
	if values[0] == "timeout" || values[0] == "t" {
		result.timeout = getInt(values[1])
	}
	if values[0] == "delimiter" || values[0] == "d" {
		result.delimiter = values[1]
	}
	if values[0] == "host" {
		result.host = values[1]
	}
	if values[0] == "service" {
		result.service = values[1]
	}
	if values[0] == "result_queue" {
		result.resultQueue = values[1]
	}
	if values[0] == "message" || values[0] == "m" {
		result.message = values[1]
	}
	if values[0] == "return_code" || values[0] == "r" {
		result.returnCode = getInt(values[1])
	}
	if values[0] == "active" {
		result.active = getBool(values[1])
	}
	if values[0] == "starttime" {
		result.startTime = getFloat(values[1])
	}
	if values[0] == "finishtime" {
		result.finishTime = getFloat(values[1])
	}
	if values[0] == "latency" {
		result.latency = getFloat(values[1])
	}
}

//opens the config file and reads all key value pairs, separated through = and commented out with #
//also reads the config files specified in the config= value
func readSettingsFile(path string, result *configurationStruct) {
	file, err := os.Open(path)

	if err != nil {
		logger.Error("config file not found")
		return
	}

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		//get line and remove whitespaces
		line := scanner.Text()
		line = strings.TrimSpace(line)
		//check if not commented out
		if len(line) > 0 && line[0] != '#' {

			//get both values
			values := strings.Split(line, "=")
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
		//check if it is an float value
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
	return float64(result)
}

func getBool(input string) bool {
	if input == "yes" || input == "on" || input == "1" {
		return true
	}
	return false
}

func bool2int(b bool) int {
	if b {
		return 1
	}
	return 0
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
