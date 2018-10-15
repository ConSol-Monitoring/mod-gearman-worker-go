package main

import (
	"os"
	"testing"
	"time"
)

func TestReadAndExecute(t *testing.T) {
	config := configurationStruct{}
	config.encryption = false
	config.jobTimeout = 30
	setDefaultValues(&config)
	thisHostname, error := os.Hostname()
	if error != nil {
		thisHostname = "unknown"
	}
	//create the test received only with needed values
	var testReceive receivedStruct
	//result_queue, core_start_time, hostname should always stay the same
	testReceive.resultQueue = "TestQueue"
	testReceive.coreTime = 123
	testReceive.hostName = "TestHost"
	testReceive.serviceDescription = "TestDescription"
	testReceive.commandLine = "sleep 1"

	resultValue := readAndExecute(&testReceive, nil, &config)

	if resultValue.source != "Mod-Gearman Worker @ "+thisHostname || resultValue.resultQueue != "TestQueue" || resultValue.coreStartTime != 123 ||
		resultValue.hostName != "TestHost" || resultValue.serviceDescription != "TestDescription" {
		t.Errorf("got %s but expected: %s", resultValue, "Mod-Gearman Worker @ "+thisHostname+"result_queue = TestQueue, core_start_time = 123, service_description = TestDescription")
	}

	//same but with identifier in config file
	config.identifier = "TestIdentifier"
	resultValue = readAndExecute(&testReceive, nil, &config)
	if resultValue.source != "Mod-Gearman Worker @ TestIdentifier" {
		t.Errorf("got %s but expected: %s", resultValue.source, "Mod-Gearman Worker @ TestIdentifier")
	}

	//check for max age
	config.maxAge = 1
	testReceive.coreTime = float64(time.Now().UnixNano())/1e9 - 2
	resultValue = readAndExecute(&testReceive, nil, &config)
	if resultValue.output != "Could not Start Check In Time" {
		t.Errorf("got %s but expected: %s", resultValue, "Could not Start Check In Time")
	}

}

func TestExecuteCommandWithTimeout(t *testing.T) {
	config := configurationStruct{}
	setDefaultValues(&config)
	config.encryption = false
	//checks needed: timeout, right return,
	returnValue, code := executeCommandWithTimeout("ls readAndExecute_test.go", 10, &config)
	if returnValue != "readAndExecute_test.go" || code != 0 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", returnValue, code, "readAndExecute_test.go", 0)
	}

	//check for timeout:
	//set return value in config
	config.timeoutReturn = 3
	returnValue, code = executeCommandWithTimeout("/bin/sleep 2", 1, &config)
	if returnValue != "timeout" || code != 4 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", returnValue, code, "timeout", 4)
	}

	//exit(3) fuer exit codes
	returnValue, code = executeCommandWithTimeout("/bin/sh -c \"exit 2\"", 10, &config)
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

func TestGetCommandBasename(t *testing.T) {
	tests := []struct {
		command string
		expect  string
	}{
		{"test", "test"},
		{"test arg1 arg2", "test"},
		{"./test", "test"},
		{"./blah/test", "test"},
		{"/test", "test"},
		{"/blah/test", "test"},
		{"/blah/test arg1 arg2", "test"},
		{"./test arg1 arg2", "test"},
		{"ENV1=1 ENV2=2 /blah/test", "test"},
		{"ENV1=1 ENV2=2 ./test", "test"},
		{"ENV1=1 ENV2=2 ./test arg1 arg2", "test"},
		{`ENV1="1 2 3" ENV2='2' ./test arg1 arg2`, "test"},
		{`PATH=test:$PATH LD_LIB=... $(pwd)/test`, "test"},
	}

	for _, test := range tests {
		base := getCommandBasename(test.command)
		if base != test.expect {
			t.Errorf("getCommandBasename was incorrect, got: %s, want: %s", base, test.expect)
		}
	}
}

//Benchmark
func BenchmarkExecuteCommandWithTimeout(b *testing.B) {
	config := configurationStruct{}
	//set the default timeout time
	config.debug = 0
	createLogger(&config)
	for n := 0; n < b.N; n++ {
		executeCommandWithTimeout("/bin/pwd", 100, &config)
	}
}

func BenchmarkReadAndExecute(b *testing.B) {
	config := configurationStruct{}
	//set the default timeout time
	config.debug = 0
	createLogger(&config)
	received := &receivedStruct{commandLine: "/bin/pwd", timeout: 10}
	for n := 0; n < b.N; n++ {
		readAndExecute(received, nil, &config)
	}
}
