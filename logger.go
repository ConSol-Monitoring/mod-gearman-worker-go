package main

import (
	"io"
	"os"
	"runtime/debug"

	"github.com/kdar/factorlog"
)

func createLogger(config *configurationStruct) {

	//logging format
	frmt := `%{Color \"yellow\" \"WARN\"}%{Color \"red\" \"ERROR\"}%{Color \"red\" \"FATAL\"}[%{Date} %{Time}][%{Severity}][%{File}:%{Line}] %{Message}`

	//check in config file if file is specified
	verbosity := getSeverity(config.debug)
	logpath := config.logfile
	var logfile io.Writer
	var err error
	logfile = os.Stdout

	if logpath != "" && (config.logmode == "automatic" || config.logmode == "file") && config.debug != 3 {
		_, err = openFileOrCreate(logpath)
		if err != nil {
			logger.Error("could not create or open file %s", logpath)
		}
		logfile, err = os.OpenFile(logpath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			logger.Error("could not open/create logfile")
		}
	}

	logger.SetFormatter(factorlog.NewStdFormatter(frmt))
	logger.SetOutput(logfile)
	logger.SetMinMaxSeverity(factorlog.StringToSeverity(verbosity), factorlog.StringToSeverity("PANIC"))

}

func getSeverity(input int) string {
	switch input {
	case 0:
		return "ERROR"
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
