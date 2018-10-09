package main

import (
	"os"
	"testing"
	"time"
)

func TestReadAndExecute(t *testing.T) {
	config.identifier = ""
	thisHostname, error := os.Hostname()
	if error != nil {
		thisHostname = "unknown"
	}
	//create the test received only with needed values
	var testReceive receivedStruct
	//result_queue, core_start_time, hostname should always stay the same
	testReceive.result_queue = "TestQueue"
	testReceive.core_time = 123
	testReceive.host_name = "TestHost"
	testReceive.service_description = "TestDescription"

	resultValue := readAndExecute(&testReceive, nil)

	if resultValue.source != "Mod-Gearman Worker @ "+thisHostname || resultValue.result_queue != "TestQueue" || resultValue.core_start_time != 123 ||
		resultValue.host_name != "TestHost" || resultValue.service_description != "TestDescription" {
		t.Errorf("got %s but expected: %s", resultValue, "Mod-Gearman Worker @ "+thisHostname+"result_queue = TestQueue, core_start_time = 123, service_description = TestDescription")
	}

	//same but with identifier in config file
	config.identifier = "TestIdentifier"
	resultValue = readAndExecute(&testReceive, nil)
	if resultValue.source != "Mod-Gearman Worker @ TestIdentifier" {
		t.Errorf("got %s but expected: %s", resultValue.source, "Mod-Gearman Worker @ TestIdentifier")
	}

	//check for max age
	config.max_age = 1
	testReceive.core_time = float64(time.Now().UnixNano())/1e9 - 2
	resultValue = readAndExecute(&testReceive, nil)
	if resultValue.output != "Could not Start Check In Time" {
		t.Errorf("got %s but expected: %s", resultValue, "Could not Start Check In Time")
	}

}

func TestExecuteCommandWithTimeout(t *testing.T) {
	//checks needed: timeout, right return,
	returnValue, code := executeCommandWithTimeout("ls readAndExecute_test.go", 10)
	if returnValue != "readAndExecute_test.go" || code != 0 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", returnValue, code, "readAndExecute_test.go", 0)
	}

	//check for timeout:
	//set return value in config
	config.timeout_return = 3
	returnValue, code = executeCommandWithTimeout("/bin/sleep 2", 1)
	if returnValue != "timeout" || code != 4 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", returnValue, code, "timeout", 4)
	}

	//exit(3) fuer exit codes
	returnValue, code = executeCommandWithTimeout("/bin/sh -c \"exit 2\"", 10)
	if returnValue != "exit status 2 " || code != 2 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", returnValue, code, "exit status 2", 5)
	}

}

func TestSplitCommandArguments(t *testing.T) {
	input := "/omd/sites/monitoring/lib/nagios/plugins/check_icmp -H 127.0.0.1 -w 3000.0,80% -c 5000.0,100% -p 5"
	expectedResult := []string{"-H", "127.0.0.1", "-w", "3000.0,80%", "-c", "5000.0,100%", "-p", "5"}

	command, returnedArgs := splitCommandArguments(input)

	if command != "/omd/sites/monitoring/lib/nagios/plugins/check_icmp" {
		t.Errorf("Error, command not extracted right, got: %s, wanted: %s", command, "/omd/sites/monitoring/lib/nagios/plugins/check_icmp")
	}

	//check if we got any results
	if len(returnedArgs) != len(expectedResult) {
		t.Errorf("sice not matching got: %d wanting: %d", len(returnedArgs), len(expectedResult))
	}

	//check if the values are matching
	for i := 0; i < len(returnedArgs); i++ {
		if returnedArgs[i] != expectedResult[i] {
			t.Errorf("splitting was incorrect, got: %s, want: %s", returnedArgs, expectedResult)
		}
	}

}

//Benchmark
func BenchmarkExecuteCommandWithTimeout(b *testing.B) {
	//set the default timeout time
	config.debug = 0
	createLogger()
	for n := 0; n < b.N; n++ {
		executeCommandWithTimeout("/bin/true", 100)
	}
}
