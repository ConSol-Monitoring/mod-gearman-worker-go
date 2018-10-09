package main

import (
	"testing"
)

func TestCheckLoads(t *testing.T) {
	min1 = 2
	min5 = 2
	min15 = 2
	config.load_limit1 = 1
	config.load_limit5 = 1
	config.load_limit15 = 1

	if checkLoads() {
		t.Errorf("loads are to high, checkload says they are right")
	}

	config.load_limit5 = 3
	config.load_limit15 = 3

	if checkLoads() {
		t.Errorf("load limit 1 exceeded")
	}

	config.load_limit1 = 3
	config.load_limit5 = 1
	config.load_limit15 = 3

	if checkLoads() {
		t.Errorf("load limit 10 exceeded")
	}

	config.load_limit5 = 3
	config.load_limit15 = 1

	if checkLoads() {
		t.Errorf("load limit 15 exceeded")
	}
}
