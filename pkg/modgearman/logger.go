package modgearman

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/kdar/factorlog"
)

// define the available log level
const (
	LogLevelInfo   = 0
	LogLevelDebug  = 1
	LogLevelTrace  = 2
	LogLevelTrace2 = 3
)

func createLogger(config *config) {
	// logging format
	frmt := `%{Color \"yellow\" \"WARN\"}` +
		`%{Color \"red\" \"ERROR\"}%{Color \"red\" \"FATAL\"}` +
		`[%{Date} %{Time "15:04:05.000"}]` +
		`[%{Severity}][%{File}:%{Line}] %{Message}`

	// check in config file if file is specified
	verbosity := getSeverity(config.debug)

	switch {
	case config.debug >= LogLevelTrace2 || config.logfile == "stderr":
		log.SetOutput(os.Stderr)
	case config.logfile != "" && (config.logmode == "automatic" || config.logmode == "file"):
		file, err := openFileOrCreate(config.logfile)
		if err != nil {
			log.Errorf("could not create or open file %s: %w", config.logfile, err)
		}
		file.Close()
		logfile, err := os.OpenFile(config.logfile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
		if err != nil {
			log.Errorf("could not open/create logfile: %w", err)
		}
		log.SetOutput(logfile)
	default:
		log.SetOutput(os.Stdout)
	}

	log.SetFormatter(factorlog.NewStdFormatter(frmt))
	log.SetMinMaxSeverity(factorlog.StringToSeverity(verbosity), factorlog.StringToSeverity("PANIC"))
	log.SetVerbosity(0)
	if config.debug >= 2 {
		log.SetVerbosity(2)
	}
}

func getSeverity(input int) string {
	if input > LogLevelTrace2 {
		input = LogLevelTrace2
	}
	switch input {
	case LogLevelInfo:
		return "INFO"
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelTrace, LogLevelTrace2:
		return "TRACE"
	}

	return "ERROR"
}

func logPanicExit() {
	if r := recover(); r != nil {
		log.Errorf("********* PANIC *********")
		log.Errorf("Panic: %s", r)
		log.Errorf("**** Stack:")
		log.Errorf("%s", debug.Stack())
		log.Errorf("*************************")
		cleanExit(1)
	}
}

// log any error with error log level
func logError(err error) {
	if err == nil {
		return
	}
	logErr := log.Output(factorlog.ERROR, 2, err.Error())
	if logErr != nil {
		fmt.Fprintf(os.Stderr, "failed to log: %s (%s)", err.Error(), logErr.Error())
	}
}

// log any error with debug log level
func logDebug(err error) {
	if err == nil {
		return
	}
	logErr := log.Output(factorlog.DEBUG, 2, err.Error())
	if logErr != nil {
		fmt.Fprintf(os.Stderr, "failed to log: %s (%s)", err.Error(), logErr.Error())
	}
}

// log any error with trace log level
func logTrace(err error) {
	if err == nil {
		return
	}
	logErr := log.Output(factorlog.TRACE, 2, err.Error())
	if logErr != nil {
		fmt.Fprintf(os.Stderr, "failed to log: %s (%s)", err.Error(), logErr.Error())
	}
}

func setLogLevel(lvl int) {
	verbosity := getSeverity(lvl)
	log.SetMinMaxSeverity(factorlog.StringToSeverity(verbosity), factorlog.StringToSeverity("PANIC"))
	log.SetVerbosity(0)
	if lvl >= 2 {
		log.SetVerbosity(2)
	}
}

func disableLogging() {
	log.SetMinMaxSeverity(factorlog.StringToSeverity("PANIC"), factorlog.StringToSeverity("PANIC"))
}
