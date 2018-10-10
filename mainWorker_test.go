package main

import (
	"testing"
)

func TestCheckLoads(t *testing.T) {
	min1 = 2
	min5 = 2
	min15 = 2
	config.loadLimit1 = 1
	config.loadLimit5 = 1
	config.loadLimit15 = 1

	if checkLoads() {
		t.Errorf("loads are to high, checkload says they are right")
	}

	config.loadLimit5 = 3
	config.loadLimit15 = 3

	if checkLoads() {
		t.Errorf("load limit 1 exceeded")
	}

	config.loadLimit1 = 3
	config.loadLimit5 = 1
	config.loadLimit15 = 3

	if checkLoads() {
		t.Errorf("load limit 10 exceeded")
	}

	config.loadLimit5 = 3
	config.loadLimit15 = 1

	if checkLoads() {
		t.Errorf("load limit 15 exceeded")
	}
}
