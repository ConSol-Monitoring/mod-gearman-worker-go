package modgearman

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/kdar/factorlog"
)

func TestCreateLogger(t *testing.T) {
	config := config{}
	file, err := os.Create("loggertest")
	if err != nil {
		t.Errorf("could not create testfile")
	}
	config.logfile = "loggertest"
	config.logmode = "automatic"
	config.debug = 2

	createLogger(&config)

	logger.Error("testError")
	logger.Debug("testDebug")

	content, _ := io.ReadAll(file)

	if !strings.Contains(string(content), "testError") {
		t.Errorf("testError not in logfile! %s", string(content))
	}

	if !strings.Contains(string(content), "testDebug") {
		t.Errorf("testDebug not in logfile! %s", string(content))
	}

	err = os.Remove("loggertest")
	if err != nil {
		t.Errorf("could not remove loggertest")
	}

	// test with file that does not exist
	logger = factorlog.New(os.Stdout, factorlog.NewStdFormatter("%{Date} %{Time} %{File}:%{Line} %{Message}"))
	createLogger(&config)
	// remove the file again
	err = os.Remove("loggertest")
	if err != nil {
		t.Errorf("could not remove loggertest")
	}

	// test logmode
	config.debug = 0 // only errors
	createLogger(&config)

	logger.Error("TestError")
	logger.Debug("TestDebug")
	logger.Info("TestInfo")

	file, err = os.Open("loggertest")
	if err != nil {
		t.Errorf("could not open loggertest File, maybe gets not created?")
	}

	content, _ = io.ReadAll(file)

	if !strings.Contains(string(content), "TestError") {
		t.Errorf("TestError not in File but should be!")
	}

	if strings.Contains(string(content), "TestDebug") {
		t.Errorf("TestDebug is in File but should not be!")
	}

	if !strings.Contains(string(content), "TestInfo") {
		t.Errorf("TestInfo is not in File but should be!")
	}

	err = os.Remove("loggertest")
	if err != nil {
		t.Errorf("could not remove loggertest")
	}
}
