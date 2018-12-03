package modgearman

import (
	"os"
	"testing"
)

func TestCheckLoads(t *testing.T) {
	if _, err := os.Stat("/proc"); os.IsNotExist(err) {
		t.Skip("skipping test without /proc/")
	}
	disableLogging()
	config := configurationStruct{}
	config.loadLimit1 = 999
	config.loadLimit5 = 999
	config.loadLimit15 = 999

	workerMap := make(map[string]*worker)
	mainworker := newMainWorker(&config, []byte("key"), &workerMap)

	if !mainworker.checkLoads() {
		t.Errorf("loads are to ok, checkload says they are too hight")
	}

	config.loadLimit1 = 0.01
	config.loadLimit5 = 999
	config.loadLimit15 = 999

	if mainworker.checkLoads() {
		t.Errorf("load limit 1 exceeded")
	}

	config.loadLimit1 = 999
	config.loadLimit5 = 0.01
	config.loadLimit15 = 999

	if mainworker.checkLoads() {
		t.Errorf("load limit 10 exceeded")
	}

	config.loadLimit1 = 999
	config.loadLimit5 = 999
	config.loadLimit15 = 0.01

	if mainworker.checkLoads() {
		t.Errorf("load limit 15 exceeded")
	}
	setLogLevel(0)
}
