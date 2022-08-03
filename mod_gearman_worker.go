package modgearman

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kdar/factorlog"
	daemon "github.com/sevlyar/go-daemon"
)

const (
	// VERSION contains the actual lmd version
	VERSION = "1.1.6"

	// ExitCodeError is used for erroneous exits
	ExitCodeError = 2

	// ExitCodeUnknown is used for unknown exits
	ExitCodeUnknown = 3

	// ConnectionRetryInterval sets the number seconds in between connection retries
	ConnectionRetryInterval = 3

	// OpenFilesBase sets the approximate number of open files excluded open files from worker
	OpenFilesBase = 50

	// OpenFilesPerWorker sets the expected number of filehandles per worker (1 gearman connection, 2 fifo pipes for stderr/stdout, one on /dev/null, one sparse)
	OpenFilesPerWorker = 5

	// OpenFilesExtraPercent adds 30% safety level when calculating required open files
	OpenFilesExtraPercent = 1.2

	// ResultServerWorker sets the number of result worker
	ResultServerWorker = 10

	// ResultServerQueueSize sets the queue size for results
	ResultServerQueueSize = 1000

	// BalooningUtilizationThreshold sets the minimum utilization in percent at where ballooning will be considered
	BalooningUtilizationThreshold = 70

	// BlockProfileRateInterval sets the profiling interval when started with -profile
	BlockProfileRateInterval = 10
)

// MainStateType is used to set different states of the main loop
type MainStateType int

const (
	// Reload flag if used after a sighup
	Reload MainStateType = iota

	// Shutdown is used when sigint received
	Shutdown

	// ShutdownGraceFully is used when sigterm received
	ShutdownGraceFully

	// Resume is used when signal does not change main state
	Resume
)

// var config configurationStruct
var logger = factorlog.New(os.Stdout, factorlog.NewStdFormatter("%{Date} %{Time} %{File}:%{Line} %{Message}"))

var prometheusListener *net.Listener
var pidFile string

// Worker starts the mod_gearman_worker program
func Worker(build string) {
	// reads the args, check if they are params, if so sends them to the configuration reader
	config, err := initConfiguration("mod_gearman_worker", build, printUsage, checkForReasonableConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
		os.Exit(ExitCodeError)
	}

	defer logPanicExit()
	if config.daemon {
		ctx := &daemon.Context{}
		d, err := ctx.Reborn()

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: unable to start daemon mode")
		}
		if d != nil {
			return
		}
		defer ctx.Release()
	}

	createPidFile(config.pidfile)
	defer deletePidFile(pidFile)

	// start usr1 routine which prints stacktraces upon request
	osSignalUsrChannel := make(chan os.Signal, 1)
	setupUsrSignalChannel(osSignalUsrChannel)
	go func() {
		defer logPanicExit()
		for {
			sig := <-osSignalUsrChannel
			mainSignalHandler(sig, config)
		}
	}()

	// initialize prometheus
	prometheusListener = startPrometheus(config)
	defer func() {
		if prometheusListener != nil {
			(*prometheusListener).Close()
		}
		logger.Infof("mod-gearman-worker-go shutdown complete")
	}()

	workerMap := make(map[string]*worker)
	initialStart := 0
	for {
		exitState, numWorker, newConfig := mainLoop(config, nil, &workerMap, initialStart)
		if exitState != Reload {
			// make it possible to call main() from tests without exiting the tests
			break
		}

		initialStart = numWorker
		if newConfig != nil {
			config = newConfig
		}
	}
}

func mainLoop(config *configurationStruct, osSignalChannel chan os.Signal, workerMap *map[string]*worker, initialStart int) (exitState MainStateType, numWorker int, newConfig *configurationStruct) {
	if osSignalChannel == nil {
		osSignalChannel = make(chan os.Signal, 1)
	}
	signal.Notify(osSignalChannel, syscall.SIGHUP)
	signal.Notify(osSignalChannel, syscall.SIGTERM)
	signal.Notify(osSignalChannel, os.Interrupt)
	signal.Notify(osSignalChannel, syscall.SIGINT)

	// create the logger, everything logged until here gets printed to stdOut
	createLogger(config)

	// create the cipher
	key := getKey(config)
	myCipher = createCipher(key, config.encryption)

	maxOpenFiles := getMaxOpenFiles()
	logger.Infof("%s - version %s (Build: %s) starting with %d workers (max %d), pid: %d (max open files: %d)\n", config.name, VERSION, config.build, config.minWorker, config.maxWorker, os.Getpid(), maxOpenFiles)

	expectedOpenFiles := uint64(float64((config.maxWorker*OpenFilesPerWorker + OpenFilesBase)) * OpenFilesExtraPercent)
	maxPossibleWorker := int(((float64(maxOpenFiles) / OpenFilesExtraPercent) - OpenFilesBase) / OpenFilesPerWorker)
	if expectedOpenFiles > maxOpenFiles {
		preMaxWorker := config.maxWorker
		config.maxWorker = maxPossibleWorker
		logger.Warnf("current max worker setting (%d) requires open files ulimit of at least %d, current value is %d. Setting max worker limit to %d", preMaxWorker, expectedOpenFiles, maxOpenFiles, config.maxWorker)
	}

	mainworker := newMainWorker(config, key, workerMap)
	mainworker.running = true
	mainworker.maxOpenFiles = maxOpenFiles
	mainworker.maxPossibleWorker = maxPossibleWorker
	defer func() { mainworker.running = false }()
	mainLoopExited := make(chan bool)

	// check connections
	go func() {
		defer logPanicExit()
		for mainworker.running {
			if mainworker.RetryFailedConnections() {
				mainworker.StopAllWorker(ShutdownGraceFully)
			}
			time.Sleep(ConnectionRetryInterval * time.Second)
		}
	}()

	// just wait till someone hits ctrl+c or we have to reload
	mainworker.manageWorkers(initialStart)
	ticker := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-ticker.C:
			mainworker.manageWorkers(0)
		case sig := <-osSignalChannel:
			exitState = mainSignalHandler(sig, config)
			switch exitState {
			case Resume:
				continue
			case Reload:
				restartRequired, config := mainworker.applyConfigChanges()
				if !restartRequired {
					// no restart of our workers required
					continue
				}
				newConfig = config
				fallthrough
			case ShutdownGraceFully:
				fallthrough
			case Shutdown:
				numWorker = len(*workerMap)
				ticker.Stop()
				// stop worker in background, so we can continue listening to signals
				go func() {
					defer logPanicExit()
					mainworker.Shutdown(exitState)
					mainLoopExited <- true
				}()
				// continue waiting for signals or an exited mainloop
				continue
			}
		case <-mainLoopExited:
			// only restart those who have exited in time
			numWorker -= len(*workerMap)
			return
		}
	}
}

type helpCallback func()
type verifyCallback func(*configurationStruct) error

func initConfiguration(name, build string, helpFunc helpCallback, verifyFunc verifyCallback) (*configurationStruct, error) {
	config := &configurationStruct{name: name, build: build}
	setDefaultValues(config)
	for i := 1; i < len(os.Args); i++ {
		// is it a param?
		if !strings.HasPrefix(os.Args[i], "--") && !strings.HasPrefix(os.Args[i], "-") {
			logger.Errorf("%s is not a param!, ignoring", os.Args[i])
			continue
		}

		arg := strings.ToLower(os.Args[i])
		switch {
		case arg == "--help" || arg == "-h":
			helpFunc()
			os.Exit(ExitCodeUnknown)
		case arg == "--version" || arg == "-v":
			printVersion(config)
			os.Exit(ExitCodeUnknown)
		case arg == "-d" || arg == "--daemon":
			config.daemon = true
		case arg == "-r":
			if len(os.Args) < i+1 {
				return nil, fmt.Errorf("-r requires an argument")
			}
			config.returnCode = getInt(os.Args[i+1])
			i++
		case arg == "-m":
			if len(os.Args) < i+1 {
				return nil, fmt.Errorf("-m requires an argument")
			}
			config.message = os.Args[i+1]
			i++
		default:
			s := strings.Trim(os.Args[i], "-")
			sa := strings.SplitN(s, "=", 2)
			if len(sa) == 1 {
				sa = append(sa, "yes")
			}
			// give the param and the value to readSetting
			readSetting(sa, config)
		}
	}
	config.removeDuplicates()
	err := verifyFunc(config)
	return config, err
}

func checkForReasonableConfig(config *configurationStruct) error {
	if len(config.server) == 0 {
		return fmt.Errorf("no server specified")
	}
	if !config.notifications && !config.services && !config.eventhandler && !config.hosts &&
		len(config.hostgroups) == 0 && len(config.servicegroups) == 0 {
		return fmt.Errorf("no listen queues defined")
	}
	if config.encryption && config.key == "" && config.keyfile == "" {
		return fmt.Errorf("encryption enabled but no keys defined")
	}

	if config.minWorker > config.maxWorker {
		config.maxWorker = config.minWorker
	}
	return nil
}

func createPidFile(path string) {
	// write the pid id if file path is defined
	if path == "" || path == "%PIDFILE%" {
		return
	}
	// check existing pid
	if checkStalePidFile(path) {
		fmt.Fprintf(os.Stderr, "Warning: removing stale pidfile %s\n", path)
	}

	err := ioutil.WriteFile(path, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0664)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Could not write pidfile: %s\n", err.Error())
		os.Exit(ExitCodeError)
	}
	pidFile = path
}

func checkStalePidFile(path string) bool {
	dat, err := ioutil.ReadFile(path)
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(dat)))
	if err != nil {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = process.Signal(syscall.Signal(0))
	if err == nil {
		fmt.Fprintf(os.Stderr, "Error: worker already running: %d\n", pid)
		os.Exit(ExitCodeError)
	}
	return true
}

func deletePidFile(f string) {
	if f != "" {
		os.Remove(f)
	}
}

func logThreaddump() {
	buf := make([]byte, 1<<16)
	n := runtime.Stack(buf, true)
	if n < len(buf) {
		buf = buf[:n]
	}
	logger.Errorf("threaddump:\n%s", buf)
}

// printVersion prints the version
func printVersion(config *configurationStruct) {
	fmt.Printf("%s - version %s (Build: %s)\n", config.name, VERSION, config.build)
}

func printUsage() {
	fmt.Print("Usage: worker [OPTION]...\n")
	fmt.Print("\n")
	fmt.Print("Mod-Gearman worker executes host- and servicechecks.\n")
	fmt.Print("\n")
	fmt.Print("Basic Settings:\n")
	fmt.Print("       --debug=<lvl>                                \n")
	fmt.Print("       --logmode=<automatic|stdout|syslog|file>     \n")
	fmt.Print("       --logfile=<path>                             \n")
	fmt.Print("       --debug-result                               \n")
	fmt.Print("       --help|-h                                    \n")
	fmt.Print("       --config=<configfile>                        \n")
	fmt.Print("       --server=<server>                            \n")
	fmt.Print("       --dupserver=<server>                         \n")
	fmt.Print("\n")
	fmt.Print("Encryption:\n")
	fmt.Print("       --encryption=<yes|no>                        \n")
	fmt.Print("       --key=<string>                               \n")
	fmt.Print("       --keyfile=<file>                             \n")
	fmt.Print("\n")
	fmt.Print("Job Control:\n")
	fmt.Print("       --hosts                                      \n")
	fmt.Print("       --services                                   \n")
	fmt.Print("       --eventhandler                               \n")
	fmt.Print("       --notifications                              \n")
	fmt.Print("       --hostgroup=<name>                           \n")
	fmt.Print("       --servicegroup=<name>                        \n")
	fmt.Print("       --max-age=<sec>                              \n")
	fmt.Print("       --job_timeout=<sec>                          \n")
	fmt.Print("\n")
	fmt.Print("Worker Control:\n")
	fmt.Print("       --min-worker=<nr>                            \n")
	fmt.Print("       --max-worker=<nr>                            \n")
	fmt.Print("       --idle-timeout=<nr>                          \n")
	fmt.Print("       --max-jobs=<nr>                              \n")
	fmt.Print("       --spawn-rate=<nr>                            \n")
	fmt.Print("       --fork_on_exec                               \n")
	fmt.Print("       --backgrounding-threshold=<sec>              \n")
	fmt.Print("       --load_limit1=load1                          \n")
	fmt.Print("       --load_limit5=load5                          \n")
	fmt.Print("       --load_limit15=load15                        \n")
	fmt.Print("       --mem_limit=<percent>                        \n")
	fmt.Print("       --show_error_output                          \n")
	fmt.Print("\n")
	fmt.Print("Worker Development:\n")
	fmt.Print("       --debug-profiler=<listen address>            \n")
	fmt.Print("       --cpuprofile=<file>                          \n")
	fmt.Print("       --memprofile=<file>                          \n")
	fmt.Print("\n")
	fmt.Print("Miscellaneous:\n")
	fmt.Print("       --workaround_rc_25\n")
	fmt.Print("\n")
	fmt.Print("see README for a detailed explanation of all options.\n")
	fmt.Print("\n")

	os.Exit(ExitCodeUnknown)
}
