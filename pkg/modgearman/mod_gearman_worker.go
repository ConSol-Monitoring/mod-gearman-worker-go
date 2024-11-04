package modgearman

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/kdar/factorlog"
	daemon "github.com/sevlyar/go-daemon"
)

const (
	// VERSION contains the actual lmd version
	VERSION = "1.5.2"

	// ExitCodeError is used for erroneous exits
	ExitCodeError = 2

	// ExitCodeUnknown is used for unknown exits
	ExitCodeUnknown = 3

	// ConnectionRetryInterval sets the number seconds in between connection retries
	ConnectionRetryInterval = 3

	// OpenFilesBase sets the approximate number of open files excluded open files from worker
	OpenFilesBase = 50

	// OpenFilesPerWorker sets the expected number of file handles per worker
	// (1 gearman connection, 2 fifo pipes for stderr/stdout, one on /dev/null, one sparse)
	OpenFilesPerWorker = 5

	// OpenFilesExtraPercent adds 30% safety level when calculating required open files
	OpenFilesExtraPercent = 1.2

	// ResultServerWorker sets the number of result worker
	ResultServerWorker = 10

	// ResultServerQueueSize sets the queue size for results
	ResultServerQueueSize = 1000

	// ballooningUtilizationThreshold sets the minimum utilization in percent at where ballooning will be considered
	ballooningUtilizationThreshold = 70

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

// LogFormat sets the log format
var LogFormat string

func init() {
	LogFormat = `[%{Date} %{Time "15:04:05.000"}]` +
		`[%{Severity}][pid:` + fmt.Sprintf("%d", os.Getpid()) + `]` +
		`[%{ShortFile}:%{Line}] %{Message}`
}

var log = factorlog.New(os.Stdout, factorlog.NewStdFormatter(LogFormat))

var (
	prometheusListener net.Listener
	pidFile            string
)

// global atomic flag wether worker should be running
var aIsRunning int64

func isRunning() bool {
	return atomic.LoadInt64(&aIsRunning) != 0
}

// Worker starts the mod_gearman_worker program
func Worker(build string) {
	// reads the args, check if they are params, if so sends them to the configuration reader
	config, err := initConfiguration("mod_gearman_worker", build, printUsage, checkForReasonableConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
		cleanExit(ExitCodeError)
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
		defer logDebug(ctx.Release())
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
			prometheusListener.Close()
		}
		log.Infof("mod-gearman-worker-go shutdown complete")
	}()

	workerMap := make(map[string]*worker)
	initialStart := 0
	for {
		exitState, numWorker, newConfig := mainLoop(config, nil, workerMap, initialStart)
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

func mainLoop(cfg *config, osSignalChan chan os.Signal, workerMap map[string]*worker, initStart int) (exit MainStateType, numWorker int, newCfg *config) {
	if osSignalChan == nil {
		osSignalChan = make(chan os.Signal, 1)
	}
	signal.Notify(osSignalChan, syscall.SIGHUP)
	signal.Notify(osSignalChan, syscall.SIGTERM)
	signal.Notify(osSignalChan, os.Interrupt)
	signal.Notify(osSignalChan, syscall.SIGINT)

	// create the logger, everything logged until here gets printed to stdOut
	createLogger(cfg)

	fileUsesEPNCache = make(map[string]EPNCacheItem)

	// create the cipher
	key := getKey(cfg)
	myCipher = createCipher(key, cfg.encryption)

	maxOpenFiles := getMaxOpenFiles()
	log.Infof("%s - version %s (Build: %s) starting with %d workers (max %d), pid: %d (max open files: %d)\n",
		cfg.binary, VERSION, cfg.build, cfg.minWorker, cfg.maxWorker, os.Getpid(), maxOpenFiles)

	expectedOpenFiles := uint64(float64((cfg.maxWorker*OpenFilesPerWorker + OpenFilesBase)) * OpenFilesExtraPercent)
	maxPossibleWorker := int(((float64(maxOpenFiles) / OpenFilesExtraPercent) - OpenFilesBase) / OpenFilesPerWorker)
	if expectedOpenFiles > maxOpenFiles {
		preMaxWorker := cfg.maxWorker
		cfg.maxWorker = maxPossibleWorker
		log.Warnf("current max worker setting (%d) requires open files ulimit of at least %d, current value is %d.",
			preMaxWorker, expectedOpenFiles, maxOpenFiles)
		log.Warnf("Setting max worker limit to %d", cfg.maxWorker)
	}

	// initialize epn sub server
	startEmbeddedPerl(cfg)
	defer stopAllEmbeddedPerl()

	mainworker := newMainWorker(cfg, key, workerMap)
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
	mainworker.manageWorkers(initStart)
	ticker := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-ticker.C:
			mainworker.manageWorkers(0)
			checkRestartEPNServer(cfg)
		case sig := <-osSignalChan:
			exit = mainSignalHandler(sig, cfg)
			switch exit {
			case Resume:
				continue
			case Reload:
				restartRequired, newConf := mainworker.applyConfigChanges()
				newCfg = newConf
				cfg = newConf
				if !restartRequired {
					// no restart of our workers required
					continue
				}

				fallthrough
			case Shutdown, ShutdownGraceFully:
				atomic.StoreInt64(&aIsRunning, 0)
				numWorker = len(workerMap)
				ticker.Stop()
				// stop worker in background, so we can continue listening to signals
				go func() {
					defer logPanicExit()
					mainworker.Shutdown(exit)
					mainLoopExited <- true
				}()
				// continue waiting for signals or an exited mainLoop
				continue
			}
		case <-mainLoopExited:
			// only restart those who have exited in time
			numWorker -= len(workerMap)

			return exit, numWorker, newCfg
		}
	}
}

type (
	helpCallback   func()
	verifyCallback func(*config) error
)

func initConfiguration(name, build string, helpFunc helpCallback, verifyFunc verifyCallback) (*config, error) {
	config := &config{binary: name, build: build}
	config.setDefaultValues()
	createLogger(config)
	for idx := 1; idx < len(os.Args); idx++ {
		if os.Args[idx] == "--" {
			break
		}

		if os.Args[idx] == "testcmd" {
			args := os.Args[idx+1:]
			if len(args) > 0 && args[0] == "--" {
				args = args[1:]
			}
			rc, out := runTestCmd(config, args)
			out = strings.TrimSpace(out)
			fmt.Fprintf(os.Stdout, "%s\n", out)
			os.Exit(rc)
		}

		// is it a param?
		if !strings.HasPrefix(os.Args[idx], "--") && !strings.HasPrefix(os.Args[idx], "-") {
			log.Errorf("%s is not a param!, ignoring", os.Args[idx])

			continue
		}

		arg := strings.ToLower(os.Args[idx])
		switch {
		case arg == "--help" || arg == "-h":
			helpFunc()
			cleanExit(ExitCodeUnknown)
		case arg == "--version" || arg == "-v":
			printVersion(config)
			cleanExit(ExitCodeUnknown)
		case arg == "-d" || arg == "--daemon":
			config.daemon = true
		case arg == "-r":
			if len(os.Args) < idx+1 {
				return nil, fmt.Errorf("-r requires an argument")
			}
			config.returnCode = getInt(os.Args[idx+1])
			idx++
		case arg == "-m":
			if len(os.Args) < idx+1 {
				return nil, fmt.Errorf("-m requires an argument")
			}
			config.message = os.Args[idx+1]
			idx++
		default:
			s := strings.TrimPrefix(strings.TrimPrefix(os.Args[idx], "-"), "-")
			if !strings.Contains(s, "=") {
				s = fmt.Sprintf("%s=yes", s)
			}
			if err := config.parseConfigItem(s); err != nil {
				return nil, fmt.Errorf("error in command line argument %s: %s", os.Args[idx], err.Error())
			}
		}
	}
	config.removeDuplicates()

	if config.debug >= LogLevelDebug {
		createLogger(config)
		config.dump()
	}

	err := verifyFunc(config)

	return config, err
}

func checkForReasonableConfig(config *config) error {
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

	if config.loadCPUMulti > 0 {
		cpuCount := runtime.NumCPU()
		if config.loadLimit1 == 0 {
			config.loadLimit1 = config.loadCPUMulti * float64(cpuCount)
		}
		if config.loadLimit5 == 0 {
			config.loadLimit5 = config.loadCPUMulti * float64(cpuCount)
		}
		if config.loadLimit15 == 0 {
			config.loadLimit15 = config.loadCPUMulti * float64(cpuCount)
		}
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

	err := os.WriteFile(path, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Could not write pidfile: %s\n", err.Error())
		cleanExit(ExitCodeError)
	}
	pidFile = path
}

func checkStalePidFile(path string) bool {
	dat, err := os.ReadFile(path)
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
		cleanExit(ExitCodeError)
	}

	return true
}

func deletePidFile(f string) {
	if f != "" {
		os.Remove(f)
	}
}

func cleanExit(exitCode int) {
	deletePidFile(pidFile)
	stopAllEmbeddedPerl()
	os.Exit(exitCode)
}

func logThreadDump() {
	buf := make([]byte, 1<<16)
	n := runtime.Stack(buf, true)
	if n < len(buf) {
		buf = buf[:n]
	}
	log.Errorf("threaddump:\n%s", buf)
}

// printVersion prints the version
func printVersion(config *config) {
	fmt.Fprintf(os.Stdout, "%s - version %s (Build: %s, %s)\n", config.binary, VERSION, config.build, runtime.Version())
}

func printUsage() {
	usage := `Usage: worker [OPTION]...

Mod-Gearman worker executes host- and servicechecks.

Basic Settings:
       --debug=<lvl>
       --logmode=<automatic|stdout|syslog|file>
       --logfile=<path>
       --debug-result
       --help|-h
       --config=<configfile>
       --server=<server>
       --dupserver=<server>

Encryption:
       --encryption=<yes|no>
       --key=<string>
       --keyfile=<file>

Job Control:
       --hosts
       --services
       --eventhandler
       --notifications
       --hostgroup=<name>
       --servicegroup=<name>
       --max-age=<sec>
       --job_timeout=<sec>

Worker Control:
       --min-worker=<nr>
       --max-worker=<nr>
       --idle-timeout=<nr>
       --max-jobs=<nr>
       --spawn-rate=<nr>
       --backgrounding-threshold=<sec>
       --load_limit1=load1
       --load_limit5=load5
       --load_limit15=load15
       --mem_limit=<percent>
       --show_error_output

Embedded Perl:
       --enable_embedded_perl=<yes|no>
       --use_embedded_perl_implicitly=<yes|no>
       --use_perl_cache=<yes|no>
       --p1_file=<path>

Worker Development:
       --debug-profiler=<listen address>
       --cpuprofile=<file>
       --memprofile=<file>

Miscellaneous:
       --workaround_rc_25

Testing Commands:

	mod_gearman_worker [--job_timeout=seconds] testcmd <cmd> <args>

see README for a detailed explanation of all options.
`
	fmt.Fprintln(os.Stdout, usage)

	cleanExit(ExitCodeUnknown)
}
