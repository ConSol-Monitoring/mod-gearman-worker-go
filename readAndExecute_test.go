package modgearman

import (
	"os"
	"strings"
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

	resultValue := readAndExecute(&testReceive, &config)

	if resultValue.source != "Mod-Gearman Worker @ "+thisHostname || resultValue.resultQueue != "TestQueue" || resultValue.coreStartTime != 123 ||
		resultValue.hostName != "TestHost" || resultValue.serviceDescription != "TestDescription" {
		t.Errorf("got %s but expected: %s", resultValue, "Mod-Gearman Worker @ "+thisHostname+"result_queue = TestQueue, core_start_time = 123, service_description = TestDescription")
	}

	//same but with identifier in config file
	config.identifier = "TestIdentifier"
	resultValue = readAndExecute(&testReceive, &config)
	if resultValue.source != "Mod-Gearman Worker @ TestIdentifier" {
		t.Errorf("got %s but expected: %s", resultValue.source, "Mod-Gearman Worker @ TestIdentifier")
	}

	//check for max age
	config.maxAge = 1
	testReceive.coreTime = float64(time.Now().UnixNano())/1e9 - 2
	resultValue = readAndExecute(&testReceive, &config)
	if resultValue.output != "Could not Start Check In Time" {
		t.Errorf("got %s but expected: %s", resultValue, "Could not Start Check In Time")
	}

}

func TestExecuteCommandWithTimeout(t *testing.T) {
	config := configurationStruct{}
	setDefaultValues(&config)
	config.encryption = false
	result := &answer{}
	//checks needed: timeout, right return,
	executeCommand(result, &receivedStruct{commandLine: "ls readAndExecute_test.go", timeout: 10}, &config)
	if result.output != "readAndExecute_test.go" || result.returnCode != 0 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "readAndExecute_test.go", 0)
	}

	//check for timeout:
	//set return value in config
	config.timeoutReturn = 3
	executeCommand(result, &receivedStruct{commandLine: "/bin/sleep 2", timeout: 1}, &config)
	if !strings.HasPrefix(result.output, "(Check Timed Out On Worker:") || result.returnCode != 3 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "timeout", 3)
	}

	//exit(3) fuer exit codes
	executeCommand(result, &receivedStruct{commandLine: "/bin/sh -c \"echo 'exit status 2'; exit 2\"", timeout: 10}, &config)
	if result.output != "exit status 2" || result.returnCode != 2 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "exit status 2", 2)
	}

	//stdout & stderr output
	executeCommand(result, &receivedStruct{commandLine: "/bin/sh -c \"echo 'stderr\nstderr' >&2; echo 'stdout\nstdout'; exit 2\"", timeout: 10}, &config)
	if result.output != `stdout\nstdout\n[stderr\nstderr]` || result.returnCode != 2 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, `stdout\nstdout\n[stderr\nstderr]`, 2)
	}

	//stderr output only
	executeCommand(result, &receivedStruct{commandLine: "/bin/sh -c \"echo 'stderr' >&2; exit 2\"", timeout: 10}, &config)
	if result.output != "[stderr]" || result.returnCode != 2 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "[stderr]", 2)
	}

	//quotes in output
	executeCommand(result, &receivedStruct{commandLine: "/bin/sh -c \"echo '\\\"'\"", timeout: 10}, &config)
	if result.output != "\"" || result.returnCode != 0 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "\"", 0)
	}

	//none-existing command
	executeCommand(result, &receivedStruct{commandLine: "/not-there \"\"", timeout: 3}, &config)
	if !strings.HasPrefix(result.output, "CRITICAL: Return code of 127 is out of bounds.") || result.returnCode != 2 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "CRITICAL: Return code of 127 is out of bounds.", 2)
	}

	//other exit codes
	executeCommand(result, &receivedStruct{commandLine: "/bin/sh -c \"echo 'exit status 42'; exit 42\"", timeout: 10}, &config)
	if !strings.HasPrefix(result.output, "CRITICAL: Return code of 42 is out of bounds.") || result.returnCode != 3 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "CRITICAL: Return code of 42 is out of bounds.", 3)
	}

	//signals
	executeCommand(result, &receivedStruct{commandLine: "/bin/sh -c \"echo 'killing\nme...'; echo 'stderr\nstderr' >&2; kill $$\"", timeout: 10}, &config)
	if !strings.HasPrefix(result.output, "CRITICAL: Return code of 15 is out of bounds.") || result.returnCode != 2 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "CRITICAL: Return code of 15 is out of bounds.", 2)
	}
	if !strings.Contains(result.output, `)\nkilling\nme...\n[stderr\nstderr]`) || result.returnCode != 2 {
		t.Errorf("got result %s, but expected containing %s", result.output, `)\nkilling\nme...\n[stderr\nstderr]`)
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
		executeCommand(&answer{}, &receivedStruct{commandLine: "/bin/pwd", timeout: 100}, &config)
	}
}

func BenchmarkReadAndExecuteShell(b *testing.B) {
	config := configurationStruct{}
	//set the default timeout time
	config.debug = 0
	createLogger(&config)
	received := &receivedStruct{commandLine: "/bin/pwd \"\"", timeout: 10}
	for n := 0; n < b.N; n++ {
		readAndExecute(received, &config)
	}
}

func BenchmarkReadAndExecuteExec(b *testing.B) {
	config := configurationStruct{}
	//set the default timeout time
	config.debug = 0
	createLogger(&config)
	received := &receivedStruct{commandLine: "/bin/pwd", timeout: 10}
	for n := 0; n < b.N; n++ {
		readAndExecute(received, &config)
	}
}
