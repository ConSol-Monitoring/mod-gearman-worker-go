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
	VERSION = "1.1.3"
)

// MainStateType is used to set different states of the main loop
type MainStateType int

const (
	// Reload flag if used after a sighup
	Reload MainStateType = iota

	// Restart is used when all worker should be recreated
	Restart

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
	defer logPanicExit()

	//reads the args, check if they are params, if so sends them to the configuration reader
	config, err := initConfiguration("mod_gearman_worker", build, printUsage, checkForReasonableConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
		os.Exit(3)
	}

	if config.daemon {
		cntxt := &daemon.Context{}
		d, err := cntxt.Reborn()

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: unable to start daemon mode")
		}
		if d != nil {
			return
		}
		defer cntxt.Release()
	}

	//create the PidFile
	createPidFile(config.pidfile)
	defer deletePidFile(pidFile)

	// start usr1 routine which prints stacktraces upon request
	osSignalUsrChannel := make(chan os.Signal, 1)
	setupUsr1Channel(osSignalUsrChannel)
	go func() {
		defer logPanicExit()
		for {
			sig := <-osSignalUsrChannel
			mainSignalHandler(sig)
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

	//create the logger, everything logged until here gets printed to stdOut
	createLogger(config)

	//create the cipher
	key := getKey(config)
	myCipher = createCipher(key, config.encryption)

	logger.Infof("%s - version %s (Build: %s) starting with %d workers (max %d), pid: %d\n", config.name, VERSION, config.build, config.minWorker, config.maxWorker, os.Getpid())
	mainworker := newMainWorker(config, key, workerMap)
	mainworker.running = true
	defer func() { mainworker.running = false }()
	mainLoopExited := make(chan bool)

	// check connections
	go func() {
		defer logPanicExit()
		for mainworker.running {
			if mainworker.RetryFailedConnections() {
				mainworker.StopAllWorker(ShutdownGraceFully)
			}
			time.Sleep(3 * time.Second)
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
			exitState = mainSignalHandler(sig)
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
					mainworker.StopAllWorker(exitState)
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
		//is it a param?
		if !strings.HasPrefix(os.Args[i], "--") && !strings.HasPrefix(os.Args[i], "-") {
			logger.Errorf("%s is not a param!, ignoring", os.Args[i])
			continue
		}

		arg := os.Args[i]
		argLc := strings.ToLower(arg)
		switch {
		case argLc == "--help" || argLc == "-h":
			helpFunc()
			os.Exit(2)
		case argLc == "--version" || argLc == "-v":
			printVersion(config)
			os.Exit(2)
		case argLc == "-d" || argLc == "--daemon":
			config.daemon = true
		case strings.HasPrefix(argLc, "-r") || strings.HasPrefix(argLc, "--returncode"):
			s := strings.Trim(arg, "-")
			sa := strings.SplitN(s, "=", 2)
			if len(sa) > 1 {
				config.returnCode, _ = strconv.Atoi(sa[1])
			} else {
				return nil, fmt.Errorf("returncode requires an argument (0-3)")
			}
		case strings.HasPrefix(argLc, "-m") || strings.HasPrefix(argLc, "--message"):
			s := strings.Trim(arg, "-")
			sa := strings.SplitN(s, "=", 2)
			if len(sa) > 1 {
				config.message = sa[1]
			} else {
				return nil, fmt.Errorf("message requires an argument")
			}
		default:
			s := strings.Trim(os.Args[i], "-")
			sa := strings.SplitN(s, "=", 2)
			if len(sa) == 1 {
				sa = append(sa, "yes")
			}
			//give the param and the value to readSetting
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
	//write the pid id if file path is defined
	if path == "" || path == "%PIDFILE%" {
		return
	}
	// check existing pid
	if dat, err := ioutil.ReadFile(path); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(dat))); err == nil {
			if process, err := os.FindProcess(pid); err == nil {
				if err := process.Signal(syscall.Signal(0)); err == nil {
					fmt.Fprintf(os.Stderr, "Error: worker already running: %d\n", pid)
					os.Exit(3)
				}
			}
		}
		fmt.Fprintf(os.Stderr, "Warning: removing stale pidfile %s\n", path)
	}
	err := ioutil.WriteFile(path, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0664)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Could not write pidfile: %s\n", err.Error())
		os.Exit(3)
	}
	pidFile = path
}

func deletePidFile(f string) {
	if f != "" {
		os.Remove(f)
	}
}

func logThreaddump() {
	buf := make([]byte, 1<<16)
	runtime.Stack(buf, true)
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
	fmt.Print("       --job_timeout=<sec>                              \n")
	fmt.Print("\n")
	fmt.Print("Worker Control:\n")
	fmt.Print("       --min-worker=<nr>                            \n")
	fmt.Print("       --max-worker=<nr>                            \n")
	fmt.Print("       --idle-timeout=<nr>                          \n")
	fmt.Print("       --max-jobs=<nr>                              \n")
	fmt.Print("       --spawn-rate=<nr>                            \n")
	fmt.Print("       --fork_on_exec                               \n")
	fmt.Print("       --load_limit1=load1                          \n")
	fmt.Print("       --load_limit5=load5                          \n")
	fmt.Print("       --load_limit15=load15                        \n")
	fmt.Print("       --show_error_output                          \n")

	fmt.Print("Miscellaneous:\n")
	fmt.Print("       --workaround_rc_25\n")
	fmt.Print("\n")
	fmt.Print("see README for a detailed explanation of all options.\n")
	fmt.Print("\n")

	os.Exit(3)
}
