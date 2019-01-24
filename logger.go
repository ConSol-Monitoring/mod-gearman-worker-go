package modgearman

import (
	"os"
	"runtime/debug"

	"github.com/kdar/factorlog"
)

func createLogger(config *configurationStruct) {

	//logging format
	frmt := `%{Color \"yellow\" \"WARN\"}%{Color \"red\" \"ERROR\"}%{Color \"red\" \"FATAL\"}[%{Date} %{Time "15:04:05.000"}][%{Severity}][%{File}:%{Line}] %{Message}`

	//check in config file if file is specified
	verbosity := getSeverity(config.debug)

	if config.debug >= 3 || config.logfile == "stderr" {
		logger.SetOutput(os.Stderr)
	} else if config.logfile != "" && (config.logmode == "automatic" || config.logmode == "file") {
		file, err := openFileOrCreate(config.logfile)
		if err != nil {
			logger.Errorf("could not create or open file %s: %s", config.logfile, err.Error())
		}
		file.Close()
		logfile, err := os.OpenFile(config.logfile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			logger.Errorf("could not open/create logfile: %s", err.Error())
		}
		logger.SetOutput(logfile)
	} else {
		logger.SetOutput(os.Stdout)
	}

	logger.SetFormatter(factorlog.NewStdFormatter(frmt))
	logger.SetMinMaxSeverity(factorlog.StringToSeverity(verbosity), factorlog.StringToSeverity("PANIC"))

}

func getSeverity(input int) string {
	switch input {
	case 0:
		return "INFO"
	case 1:
		return "DEBUG"
	case 2:
		return "TRACE"
	case 3:
		return "TRACE"
	}
	return "ERROR"
}

func logPanicExit() {
	if r := recover(); r != nil {
		logger.Errorf("********* PANIC *********")
		logger.Errorf("Panic: %s", r)
		logger.Errorf("**** Stack:")
		logger.Errorf("%s", debug.Stack())
		logger.Errorf("*************************")
		os.Exit(1)
	}
}

func setLogLevel(lvl int) {
	verbosity := getSeverity(lvl)
	logger.SetMinMaxSeverity(factorlog.StringToSeverity(verbosity), factorlog.StringToSeverity("PANIC"))
}

func disableLogging() {
	logger.SetMinMaxSeverity(factorlog.StringToSeverity("PANIC"), factorlog.StringToSeverity("PANIC"))
}
