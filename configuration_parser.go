package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

type configurationStruct struct {
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
	minWorker                 int //special
	maxWorker                 int //special
	idleTimeout               int //special
	maxAge                    int //special
	spawnRate                 int //special
	forkOnExec                bool
	loadLimit1                float32
	loadLimit5                float32
	loadLimit15               float32
	showErrorOutput           bool
	dupResultsArePassive      bool
	enableEmbeddedPerl        bool
	useEmbeddedPerlImplicitly bool
	usePerlCache              bool
	p1File                    string
	gearmanConnectionTimeout  int
	restrictPath              []string
	restrictCommandCharacters []string
	workaroundRc25            bool
	server                    []string
	timeoutReturn             int
	daemon                    bool
	prometheusServer          string
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
	if values[0] == "restrict_command_characters" {
		// result.restrict_command_characters = values[1]
		for _, v := range values[1] {
			result.restrictCommandCharacters = append(result.restrictCommandCharacters, string(v))
		}
	}
	if values[0] == "workaround_rc_25" {
		result.workaroundRc25 = getBool(values[1])
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

func getFloat(input string) float32 {
	if input == "" {
		return float32(0)
	}
	result, err := strconv.ParseFloat(input, 32)
	if err != nil {
		logger.Errorf("error Converting %s to float", input)
		result = 0
	}
	return float32(result)
}

func getBool(input string) bool {
	if input == "yes" || input == "on" || input == "1" {
		return true
	}
	return false
}
