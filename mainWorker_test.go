package main

import (
	"testing"
)

func TestCheckLoads(t *testing.T) {
	disableLogging()
	config := configurationStruct{}
	config.loadLimit1 = 999
	config.loadLimit5 = 999
	config.loadLimit15 = 999

	mainworker := newMainWorker(&config, []byte("key"))

	if !mainworker.checkLoads() {
		t.Errorf("loads are to ok, checkload says they are too hight")
	}

	config.loadLimit5 = 0.01
	config.loadLimit15 = 0.01

	if mainworker.checkLoads() {
		t.Errorf("load limit 1 exceeded")
	}

	config.loadLimit1 = 3
	config.loadLimit5 = 1
	config.loadLimit15 = 3

	if mainworker.checkLoads() {
		t.Errorf("load limit 10 exceeded")
	}

	config.loadLimit5 = 3
	config.loadLimit15 = 1

	if mainworker.checkLoads() {
		t.Errorf("load limit 15 exceeded")
	}
	setLogLevel(0)
}
