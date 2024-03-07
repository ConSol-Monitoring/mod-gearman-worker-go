package modgearman

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/kdar/factorlog"
)

func TestCreateLogger(t *testing.T) {
	cfg := config{}
	file, err := os.Create("loggertest")
	if err != nil {
		t.Errorf("could not create testfile")
	}
	cfg.logfile = "loggertest"
	cfg.logmode = "automatic"
	cfg.debug = 2

	createLogger(&cfg)

	log.Error("testError")
	log.Debug("testDebug")

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
	log = factorlog.New(os.Stdout, factorlog.NewStdFormatter("%{Date} %{Time} %{File}:%{Line} %{Message}"))
	createLogger(&cfg)
	// remove the file again
	err = os.Remove("loggertest")
	if err != nil {
		t.Errorf("could not remove loggertest")
	}

	// test logmode
	cfg.debug = 0 // only errors
	createLogger(&cfg)

	log.Error("TestError")
	log.Debug("TestDebug")
	log.Info("TestInfo")

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
