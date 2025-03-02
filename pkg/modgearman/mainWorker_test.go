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
	cfg := config{}
	cfg.loadLimit1 = 999
	cfg.loadLimit5 = 999
	cfg.loadLimit15 = 999

	workerMap := make(map[string]*worker)
	mainworker := newMainWorker(&cfg, []byte("key"), workerMap)

	mainworker.updateLoadAvg()
	passed, _ := mainworker.checkLoads()
	if !passed {
		t.Errorf("loads are to ok, checkload says they are too high")
	}

	cfg.loadLimit1 = 0.01
	cfg.loadLimit5 = 999
	cfg.loadLimit15 = 999

	passed, _ = mainworker.checkLoads()
	if passed {
		t.Errorf("load limit 1 exceeded")
	}

	cfg.loadLimit1 = 999
	cfg.loadLimit5 = 0.01
	cfg.loadLimit15 = 999

	passed, _ = mainworker.checkLoads()
	if passed {
		t.Errorf("load limit 10 exceeded")
	}

	cfg.loadLimit1 = 999
	cfg.loadLimit5 = 999
	cfg.loadLimit15 = 0.01

	passed, _ = mainworker.checkLoads()
	if passed {
		t.Errorf("load limit 15 exceeded")
	}
	setLogLevel(0)
}
