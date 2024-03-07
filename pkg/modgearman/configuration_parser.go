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

type configurationStruct struct {
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
	memLimit                  int
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
	internalNegate      bool
	internalCheckDummy  bool
	internalCheckNscWeb bool
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
	workerNameInResult bool
}

// setDefaultValues sets reasonable defaults
func (config *configurationStruct) setDefaultValues() {
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
	config.workerNameInResult = false
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

// removeDuplicates removes duplicate entries from all string lists
func (config *configurationStruct) removeDuplicates() {
	config.server = removeDuplicateStrings(config.server)
	config.dupserver = removeDuplicateStrings(config.dupserver)
	config.hostgroups = removeDuplicateStrings(config.hostgroups)
	config.servicegroups = removeDuplicateStrings(config.servicegroups)
	config.restrictPath = removeDuplicateStrings(config.restrictPath)
}

// dump logs all config items
func (config *configurationStruct) dump() {
	logger.Debugf("binary                        %s\n", config.binary)
	logger.Debugf("build                         %s\n", config.build)
	logger.Debugf("identifier                    %s\n", config.identifier)
	logger.Debugf("debug                         %d\n", config.debug)
	logger.Debugf("logfile                       %s\n", config.logfile)
	logger.Debugf("logmode                       %s\n", config.logmode)
	logger.Debugf("server                        %v\n", config.server)
	logger.Debugf("dupserver                     %v\n", config.dupserver)
	logger.Debugf("eventhandler                  %v\n", config.eventhandler)
	logger.Debugf("notifications                 %v\n", config.notifications)
	logger.Debugf("services                      %v\n", config.services)
	logger.Debugf("hosts                         %v\n", config.hosts)
	logger.Debugf("hostgroups                    %v\n", config.hostgroups)
	logger.Debugf("servicegroups                 %v\n", config.servicegroups)
	logger.Debugf("encryption                    %v\n", config.encryption)
	logger.Debugf("keyfile                       %s\n", config.keyfile)
	logger.Debugf("pidfile                       %s\n", config.pidfile)
	logger.Debugf("jobTimeout                    %ds\n", config.jobTimeout)
	logger.Debugf("minWorker                     %d\n", config.minWorker)
	logger.Debugf("maxWorker                     %d\n", config.maxWorker)
	logger.Debugf("idleTimeout                   %ds\n", config.idleTimeout)
	logger.Debugf("maxAge                        %d\n", config.maxAge)
	logger.Debugf("spawnRate                     %d/s\n", config.spawnRate)
	logger.Debugf("sinkRate                      %d/s\n", config.sinkRate)
	logger.Debugf("loadLimit1                    %.2f\n", config.loadLimit1)
	logger.Debugf("loadLimit5                    %.2f\n", config.loadLimit5)
	logger.Debugf("loadLimit15                   %.2f\n", config.loadLimit15)
	logger.Debugf("loadCPUMulti                  %.2f\n", config.loadCPUMulti)
	logger.Debugf("memLimit                      %d%%\n", config.memLimit)
	logger.Debugf("backgroundingThreshold        %ds\n", config.backgroundingThreshold)
	logger.Debugf("showErrorOutput               %v\n", config.showErrorOutput)
	logger.Debugf("dupResultsArePassive          %v\n", config.dupResultsArePassive)
	logger.Debugf("dupServerBacklogQueueSize     %d\n", config.dupServerBacklogQueueSize)
	logger.Debugf("restrictPath                  %v\n", config.restrictPath)
	logger.Debugf("timeoutReturn                 %d\n", config.timeoutReturn)
	logger.Debugf("daemon                        %v\n", config.daemon)
	logger.Debugf("prometheusServer              %s\n", config.prometheusServer)
	logger.Debugf("enableEmbeddedPerl            %v\n", config.enableEmbeddedPerl)
	logger.Debugf("useEmbeddedPerlImplicitly     %v\n", config.useEmbeddedPerlImplicitly)
	logger.Debugf("usePerlCache                  %v\n", config.usePerlCache)
	logger.Debugf("p1File                        %s\n", config.p1File)
	logger.Debugf("internal_negate               %v\n", config.internalNegate)
	logger.Debugf("internal_check_dummy          %v\n", config.internalCheckDummy)
	logger.Debugf("internal_check_nsc_web        %v\n", config.internalCheckNscWeb)
	logger.Debugf("worker_name_in_result         %v\n", config.workerNameInResult)
	if config.binary == "send_gearman" {
		logger.Debugf("returncode                    %d\n", config.returnCode)
		logger.Debugf("message                       %s\n", config.message)
		logger.Debugf("host                          %s\n", config.host)
		logger.Debugf("service                       %s\n", config.service)
	}
}

// parses the key value pairs and stores them in the configuration struct
func (config *configurationStruct) parseConfigItem(raw string) error {
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
		for i := 0; i < len(list); i++ {
			list[i] = strings.Trim(list[i], " ")
		}
		config.hostgroups = append(config.hostgroups, list...)
	case "servicegroups":
		list := strings.Split(value, ",")
		for i := 0; i < len(list); i++ {
			list[i] = strings.Trim(list[i], " ")
		}
		config.servicegroups = append(config.servicegroups, list...)
	case "server":
		list := strings.Split(value, ",")
		for i := 0; i < len(list); i++ {
			list[i] = strings.Trim(list[i], " ")
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
		config.memLimit = getInt(value)
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
	case "worker_name_in_result":
		config.workerNameInResult = getBool(value)
	case "fork_on_exec":
		// skip legacy option
	case "workaround_rc_25":
		// skip legacy option
	default:
		logger.Warnf("unknown configuration option: %s", raw)
	}

	return nil
}

// read settings from file or folder
func (config *configurationStruct) readSettingsPath(path string) error {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot read %s: %s", path, err.Error())
	}

	if fileInfo.IsDir() {
		err = filepath.Walk(path, func(path string, info fs.FileInfo, _ error) error {
			if info.IsDir() {
				return nil
			}

			if !strings.HasSuffix(path, ".cfg") && !strings.HasSuffix(path, ".conf") {
				return nil
			}

			return config.readSettingsFile(path)
		})
		if err != nil {
			return fmt.Errorf("error reading configuration from %s: %s", path, err.Error())
		}
	}

	return config.readSettingsFile(path)
}

// opens the config file and reads all key value pairs, separated through = and commented out with #
// also reads the config files specified in the config= value
func (config *configurationStruct) readSettingsFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot read file %s: %s", path, err.Error())
	}

	scanner := bufio.NewScanner(file)

	nr := 0
	for scanner.Scan() {
		nr++
		// get line and remove whitespaces
		line := scanner.Text()
		line = strings.TrimSpace(line)

		// skip empty lines
		if len(line) == 0 {
			continue
		}

		// skip comments
		if line[0] == '#' {
			continue
		}

		if err := config.parseConfigItem(line); err != nil {
			return fmt.Errorf("parse error in file %s:%d: %s", path, nr, err.Error())
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
