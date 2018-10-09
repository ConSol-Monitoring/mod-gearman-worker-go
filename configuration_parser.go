package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

type configurationStruct struct {
	identifier                   string
	debug                        int
	logfile                      string
	logmode                      string
	config                       []string
	dupserver                    []string
	eventhandler                 bool
	notifications                bool
	services                     bool
	hosts                        bool
	hostgroups                   []string
	servicegroups                []string
	encryption                   bool
	key                          string
	keyfile                      string
	pidfile                      string
	job_timeout                  int
	min_worker                   int //special
	max_worker                   int //special
	idle_timeout                 int //special
	max_jobs                     int //special
	max_age                      int //special
	spawn_rate                   int //special
	fork_on_exec                 bool
	load_limit1                  float32
	load_limit5                  float32
	load_limit15                 float32
	show_error_output            bool
	dup_results_are_passive      bool
	enable_embedded_perl         bool
	use_embedded_perl_implicitly bool
	use_perl_cache               bool
	p1_file                      string
	gearman_connection_timeout   int
	restrict_path                []string
	restrict_command_characters  []string
	workaround_rc_25             bool
	server                       []string
	timeout_return               int
	daemon                       bool
	prometheus_server            string
}

func setDefaultValues(result *configurationStruct) {
	result.logmode = "automatic"
	result.encryption = true
	result.show_error_output = true
	result.debug = 1
	result.logmode = "automatic"
	result.dup_results_are_passive = true
	result.gearman_connection_timeout = -1
	result.timeout_return = 3
	result.idle_timeout = 60
	result.daemon = false
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
		result.prometheus_server = values[1]
	}
	if values[0] == "timeout_return" {
		result.timeout_return = getInt(values[1])
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
		result.job_timeout = getInt(values[1])
	}
	if values[0] == "min-worker" {
		result.min_worker = getInt(values[1])
	}
	if values[0] == "max-worker" {
		result.max_worker = getInt(values[1])
	}
	if values[0] == "idle-timeout" {
		result.idle_timeout = getInt(values[1])
	}
	if values[0] == "max-jobs" {
		result.max_jobs = getInt(values[1])
	}
	if values[0] == "max-age" {
		result.max_age = getInt(values[1])
	}
	if values[0] == "spawn-rate" {
		result.spawn_rate = getInt(values[1])
	}
	if values[0] == "fork_on_exec" {
		result.fork_on_exec = getBool(values[1])
	}
	if values[0] == "load_limit1" {
		result.load_limit1 = getFloat(values[1])
	}
	if values[0] == "load_limit5" {
		result.load_limit5 = getFloat(values[1])
	}
	if values[0] == "load_limit15" {
		result.load_limit15 = getFloat(values[1])
	}
	if values[0] == "show_error_output" {
		result.show_error_output = getBool(values[1])
	}
	if values[0] == "dup_results_are_passive" {
		result.dup_results_are_passive = getBool(values[1])
	}
	if values[0] == "enable_embedded_perl" {
		result.enable_embedded_perl = getBool(values[1])
	}
	if values[0] == "use_embedded_perl_implicitly" {
		result.use_embedded_perl_implicitly = getBool(values[1])
	}
	if values[0] == "use_perl_cache" {
		result.use_perl_cache = getBool(values[1])
	}
	if values[0] == "p1_file" {
		result.p1_file = values[1]
	}
	if values[0] == "gearman_connection_timeout" {
		result.gearman_connection_timeout = getInt(values[1])
	}
	if values[0] == "restrict_path" {
		result.restrict_path = append(result.restrict_path, values[1])
	}
	if values[0] == "restrict_command_characters" {
		// result.restrict_command_characters = values[1]
		for _, v := range values[1] {
			result.restrict_command_characters = append(result.restrict_command_characters, string(v))
		}
	}
	if values[0] == "workaround_rc_25" {
		result.workaround_rc_25 = getBool(values[1])
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
	result, err := strconv.Atoi(input)
	if err != nil {
		//check if it is an float value
		result = int(getFloat(input))
		logger.Error("Error converting " + input + " to int, try with float")
	}
	return result
}

func getFloat(input string) float32 {
	result, err := strconv.ParseFloat(input, 32)
	if err != nil {
		logger.Error("error Converting ", input)
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
