package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/kdar/factorlog"
	daemon "github.com/sevlyar/go-daemon"
)

// Build contains the current git commit id
// compile passing -ldflags "-X main.Build <build sha1>" to set the id.
var Build string

const (
	// VERSION contains the actual lmd version
	VERSION = "0.0.1"
	// NAME defines the name of this project
	NAME = "mod-gearman-worker-go"
)

var config configurationStruct
var logger = factorlog.New(os.Stdout, factorlog.NewStdFormatter("%{Date} %{Time} %{File}:%{Line} %{Message}"))
var key []byte

func main() {

	setDefaultValues(&config)

	//reads the args, check if they are params, if so sends them to the configuration reader
	if len(os.Args) > 1 {
		for i := 1; i < len(os.Args); i++ {
			//is it a param?
			if strings.HasPrefix(os.Args[i], "--") || strings.HasPrefix(os.Args[i], "-") {
				if os.Args[i] == "--help" || os.Args[i] == "-h" {
					printUsage()
				} else if os.Args[i] == "--version" || os.Args[i] == "-v" {
					printVersion()
					os.Exit(3)
				} else if os.Args[i] == "-d" || os.Args[i] == "--daemon" {
					config.daemon = true
				} else {
					s := strings.Trim(os.Args[i], "--")
					sa := strings.Split(s, "=")
					if len(sa) == 1 {
						sa = append(sa, "yes")
					}
					//give the param and the value to readSetting
					readSetting(sa, &config)
				}
			} else {
				fmt.Println(os.Args[i] + " is not a param!, ignoring")
			}
		}
	} else {
		fmt.Println("Missing Parameters")
		printUsage()
	}

	go startPrometheus()

	startDaemonIfConfigured()

	//set the key
	key = getKey()

	//create the PidFile
	createPidFile()

	//create the logger, everything logged until here gets printed to stdOut
	createLogger()

	//create de cipher
	myCipher = createCipher()

	logger.Debugf("%s - version %s (Build: %s) starting with %d workers (max %d)\n", NAME, VERSION, Build, config.minWorker, config.maxWorker)
	startMinWorkers()

}

func startDaemonIfConfigured() {
	if config.daemon {
		cntxt := &daemon.Context{}
		d, err := cntxt.Reborn()

		if err != nil {
			logger.Error("unable to start daemon mode")
		}
		if d != nil {
			return
		}
		defer cntxt.Release()
	}

}

func createPidFile() {
	//write the pid id if file path is defined
	if config.pidfile != "" && config.pidfile != "%PIDFILE%" {
		f, err := os.Create(config.pidfile)
		if err != nil {
			logger.Error("Could not open/create Pidfile!!")
		} else {
			f.WriteString(strconv.Itoa(os.Getpid()))
			defer deletePidFile(config.pidfile)
		}

	}
}

func deletePidFile(f string) {
	os.Remove(f)
}

// printVersion prints the version
func printVersion() {
	fmt.Printf("%s - version %s (Build: %s)\n", NAME, VERSION, Build)
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
