package modgearman

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

type config struct {
	binary                    string
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
	loadCPUMulti              float64
	memLimit                  uint64
	backgroundingThreshold    int
	showErrorOutput           bool
	dupResultsArePassive      bool
	dupServerBacklogQueueSize int
	restrictPath              []string
	server                    []string
	timeoutReturn             int
	daemon                    bool
	prometheusServer          string
	enableEmbeddedPerl        bool
	useEmbeddedPerlImplicitly bool
	usePerlCache              bool
	p1File                    string
	// internal plugins
	internalNegate          bool
	internalCheckDummy      bool
	internalCheckNscWeb     bool
	internalCheckPrometheus bool
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
	// append worker hostname to result output
	workerNameInResult string
}

// setDefaultValues sets reasonable defaults
func (config *config) setDefaultValues() {
	config.logmode = "automatic"
	config.encryption = true
	config.showErrorOutput = true
	config.debug = 0
	config.logmode = "automatic"
	config.dupResultsArePassive = true
	config.dupServerBacklogQueueSize = 1000
	config.timeoutReturn = 3
	config.jobTimeout = 60
	config.idleTimeout = 10
	config.daemon = false
	config.minWorker = 1
	config.maxWorker = 20
	config.spawnRate = 3
	config.sinkRate = 1
	config.backgroundingThreshold = 30
	config.loadCPUMulti = 2.5
	config.memLimit = 70
	config.enableEmbeddedPerl = false
	config.useEmbeddedPerlImplicitly = false
	config.usePerlCache = true
	config.internalNegate = true
	config.internalCheckDummy = true
	config.internalCheckNscWeb = true
	config.internalCheckPrometheus = true
	config.workerNameInResult = "off"
	filename, err := os.Executable()
	if err == nil {
		config.p1File = path.Join(path.Dir(filename), "mod_gearman_worker_epn.pl")
	}
	hostname, _ := os.Hostname()
	config.identifier = hostname
	if config.identifier == "" {
		config.identifier = "unknown"
	}
	config.delimiter = "\t"
}

// cleanListAttributes removes duplicate and empty entries from all string lists
func (config *config) cleanListAttributes() {
	config.server = cleanListAttribute(config.server)
	config.dupserver = cleanListAttribute(config.dupserver)
	config.hostgroups = cleanListAttribute(config.hostgroups)
	config.servicegroups = cleanListAttribute(config.servicegroups)
	config.restrictPath = cleanListAttribute(config.restrictPath)
}

// dump logs all config items
func (config *config) dump() {
	log.Debugf("binary                        %s\n", config.binary)
	log.Debugf("build                         %s\n", config.build)
	log.Debugf("identifier                    %s\n", config.identifier)
	log.Debugf("debug                         %d\n", config.debug)
	log.Debugf("logfile                       %s\n", config.logfile)
	log.Debugf("logmode                       %s\n", config.logmode)
	log.Debugf("server                        %v\n", config.server)
	log.Debugf("dupserver                     %v\n", config.dupserver)
	log.Debugf("eventhandler                  %v\n", config.eventhandler)
	log.Debugf("notifications                 %v\n", config.notifications)
	log.Debugf("services                      %v\n", config.services)
	log.Debugf("hosts                         %v\n", config.hosts)
	log.Debugf("hostgroups                    %v\n", config.hostgroups)
	log.Debugf("servicegroups                 %v\n", config.servicegroups)
	log.Debugf("encryption                    %v\n", config.encryption)
	log.Debugf("keyfile                       %s\n", config.keyfile)
	log.Debugf("pidfile                       %s\n", config.pidfile)
	log.Debugf("jobTimeout                    %ds\n", config.jobTimeout)
	log.Debugf("minWorker                     %d\n", config.minWorker)
	log.Debugf("maxWorker                     %d\n", config.maxWorker)
	log.Debugf("idleTimeout                   %ds\n", config.idleTimeout)
	log.Debugf("maxAge                        %d\n", config.maxAge)
	log.Debugf("spawnRate                     %d/s\n", config.spawnRate)
	log.Debugf("sinkRate                      %d/s\n", config.sinkRate)
	log.Debugf("loadLimit1                    %.2f\n", config.loadLimit1)
	log.Debugf("loadLimit5                    %.2f\n", config.loadLimit5)
	log.Debugf("loadLimit15                   %.2f\n", config.loadLimit15)
	log.Debugf("loadCPUMulti                  %.2f\n", config.loadCPUMulti)
	log.Debugf("memLimit                      %d%%\n", config.memLimit)
	log.Debugf("backgroundingThreshold        %ds\n", config.backgroundingThreshold)
	log.Debugf("showErrorOutput               %v\n", config.showErrorOutput)
	log.Debugf("dupResultsArePassive          %v\n", config.dupResultsArePassive)
	log.Debugf("dupServerBacklogQueueSize     %d\n", config.dupServerBacklogQueueSize)
	log.Debugf("restrictPath                  %v\n", config.restrictPath)
	log.Debugf("timeoutReturn                 %d\n", config.timeoutReturn)
	log.Debugf("daemon                        %v\n", config.daemon)
	log.Debugf("prometheusServer              %s\n", config.prometheusServer)
	log.Debugf("enableEmbeddedPerl            %v\n", config.enableEmbeddedPerl)
	log.Debugf("useEmbeddedPerlImplicitly     %v\n", config.useEmbeddedPerlImplicitly)
	log.Debugf("usePerlCache                  %v\n", config.usePerlCache)
	log.Debugf("p1File                        %s\n", config.p1File)
	log.Debugf("internal_negate               %v\n", config.internalNegate)
	log.Debugf("internal_check_dummy          %v\n", config.internalCheckDummy)
	log.Debugf("internal_check_nsc_web        %v\n", config.internalCheckNscWeb)
	log.Debugf("internal_check_prometheus     %v\n", config.internalCheckPrometheus)
	log.Debugf("worker_name_in_result         %v\n", config.workerNameInResult)
	if config.binary == "send_gearman" {
		log.Debugf("returncode                    %d\n", config.returnCode)
		log.Debugf("message                       %s\n", config.message)
		log.Debugf("host                          %s\n", config.host)
		log.Debugf("service                       %s\n", config.service)
	}
}

// parses the key value pairs and stores them in the configuration struct
func (config *config) parseConfigItem(raw string) error {
	values := strings.SplitN(raw, "=", 2)
	if len(values) <= 1 {
		return fmt.Errorf("parse error, expected key=value in %s", raw)
	}
	key := strings.ToLower(strings.Trim(values[0], " "))
	value := strings.Trim(values[1], " ")

	switch key {
	case "dupserver":
		list := strings.Split(value, ",")
		config.dupserver = append(config.dupserver, list...)
	case "hostgroups":
		list := strings.Split(value, ",")
		for i, el := range list {
			list[i] = strings.Trim(el, " ")
		}
		config.hostgroups = append(config.hostgroups, list...)
	case "servicegroups":
		list := strings.Split(value, ",")
		for i, el := range list {
			list[i] = strings.Trim(el, " ")
		}
		config.servicegroups = append(config.servicegroups, list...)
	case "server":
		list := strings.Split(value, ",")
		for i, el := range list {
			list[i] = strings.Trim(el, " ")
		}
		for i, s := range list {
			list[i] = fixGearmandServerAddress(s)
		}
		config.server = append(config.server, list...)
	case "prometheus_server":
		config.prometheusServer = value
	case "timeout_return":
		config.timeoutReturn = getInt(value)
	case "config":
		err := config.readSettingsPath(value)
		if err != nil {
			return err
		}
	case "debug":
		config.debug = getInt(value)
		if config.debug > LogLevelTrace2 {
			config.debug = LogLevelTrace2
		}
		createLogger(config)
	case "logfile":
		config.logfile = value
		createLogger(config)
	case "logmode":
		config.logmode = value
		createLogger(config)
	case "identifier":
		config.identifier = value
	case "eventhandler":
		config.eventhandler = getBool(value)
	case "notifications":
		config.notifications = getBool(value)
	case "services":
		config.services = getBool(value)
	case "hosts":
		config.hosts = getBool(value)
	case "encryption":
		config.encryption = getBool(value)
	case "key":
		config.key = value
	case "keyfile":
		config.keyfile = value
	case "pidfile":
		config.pidfile = value
	case "job_timeout":
		config.jobTimeout = getInt(value)
	case "min-worker":
		config.minWorker = getInt(value)
	case "max-worker":
		config.maxWorker = getInt(value)
	case "idle-timeout":
		config.idleTimeout = getInt(value)
	case "max-age":
		config.maxAge = getInt(value)
	case "spawn-rate":
		config.spawnRate = getInt(value)
	case "sink-rate":
		config.sinkRate = getInt(value)
	case "load_limit1":
		config.loadLimit1 = getFloat(value)
	case "load_limit5":
		config.loadLimit5 = getFloat(value)
	case "load_limit15":
		config.loadLimit15 = getFloat(value)
	case "load_cpu_multi":
		config.loadCPUMulti = getFloat(value)
	case "mem_limit":
		config.memLimit = uint64(getFloat(value))
	case "backgrounding-threshold":
		config.backgroundingThreshold = getInt(value)
	case "show_error_output":
		config.showErrorOutput = getBool(value)
	case "dup_results_are_passive":
		config.dupResultsArePassive = getBool(value)
	case "dupserver_backlog_queue_size":
		config.dupServerBacklogQueueSize = getInt(value)
	case "gearman_connection_timeout":
		// unused, timeout is not exposed by libworker
	case "max-jobs":
		// unused
	case "restrict_path":
		config.restrictPath = append(config.restrictPath, value)
	case "timeout", "t":
		config.timeout = getInt(value)
	case "delimiter", "d":
		config.delimiter = value
	case "host":
		config.host = value
	case "service":
		config.service = value
	case "result_queue":
		config.resultQueue = value
	case "message", "m":
		config.message = value
	case "return_code", "r":
		config.returnCode = getInt(value)
	case "active":
		config.active = getBool(value)
	case "starttime":
		config.startTime = getFloat(value)
	case "finishtime":
		config.finishTime = getFloat(value)
	case "latency":
		config.latency = getFloat(value)
	case "debug-profiler":
		config.flagProfile = value
	case "cpuprofile":
		config.flagCPUProfile = value
	case "memprofile":
		config.flagMemProfile = value
	case "enable_embedded_perl":
		config.enableEmbeddedPerl = getBool(value)
	case "use_embedded_perl_implicitly":
		config.useEmbeddedPerlImplicitly = getBool(value)
	case "use_perl_cache":
		config.usePerlCache = getBool(value)
	case "p1_file":
		config.p1File = value
	case "internal_negate":
		config.internalNegate = getBool(value)
	case "internal_check_dummy":
		config.internalCheckDummy = getBool(value)
	case "internal_check_nsc_web":
		config.internalCheckNscWeb = getBool(value)
	case "internal_check_prometheus":
		config.internalCheckPrometheus = getBool(value)
	case "worker_name_in_result":
		config.workerNameInResult = value
	case "fork_on_exec":
		// skip legacy option
	case "workaround_rc_25":
		// skip legacy option
	default:
		log.Warnf("unknown configuration option: %s", raw)
	}

	return nil
}

// read settings from file or folder
func (config *config) readSettingsPath(filename string) error {
	fileInfo, err := os.Stat(filename)
	if err != nil {
		return fmt.Errorf("cannot read %s: %s", filename, err.Error())
	}

	if fileInfo.IsDir() {
		err = filepath.Walk(filename, func(path string, info fs.FileInfo, _ error) error {
			if info.IsDir() {
				return nil
			}

			if !strings.HasSuffix(path, ".cfg") && !strings.HasSuffix(path, ".conf") {
				return nil
			}

			return config.readSettingsFile(path)
		})
		if err != nil {
			return fmt.Errorf("error reading configuration from %s: %s", filename, err.Error())
		}
	}

	return config.readSettingsFile(filename)
}

// opens the config file and reads all key value pairs, separated through = and commented out with #
// also reads the config files specified in the config= value
func (config *config) readSettingsFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("cannot read file %s: %s", filename, err.Error())
	}

	scanner := bufio.NewScanner(file)

	lineNr := 0
	for scanner.Scan() {
		lineNr++
		// get line and remove whitespaces
		line := scanner.Text()
		line = strings.TrimSpace(line)

		// skip empty lines
		if line == "" {
			continue
		}

		// skip comments
		if line[0] == '#' {
			continue
		}

		if err := config.parseConfigItem(line); err != nil {
			return fmt.Errorf("parse error in file %s:%d: %s", filename, lineNr, err.Error())
		}
	}

	return nil
}

func getInt(input string) int {
	if input == "" {
		return 0
	}
	result, err := strconv.Atoi(input)
	if err != nil {
		// check if it is an float value
		log.Debugf("Error converting %s to int, try with float", input)
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
		log.Errorf("error Converting %s to float", input)
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

// cleanListAttribute removes duplicate and empty entries from all string lists
func cleanListAttribute(elements []string) []string {
	encountered := map[string]bool{}
	uniq := []string{}

	for _, s := range elements {
		if s != "" && !encountered[s] {
			encountered[s] = true
			uniq = append(uniq, s)
		}
	}

	return uniq
}
