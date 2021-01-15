package modgearman

import (
	"bytes"
	"math"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestReadAndExecute(t *testing.T) {
	config := configurationStruct{}
	config.encryption = false
	config.jobTimeout = 30
	setDefaultValues(&config)
	thisHostname, err := os.Hostname()
	if err != nil {
		thisHostname = "unknown"
	}
	// create the test received only with needed values
	var testReceive receivedStruct
	// result_queue, core_start_time, hostname should always stay the same
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

	// same but with identifier in config file
	config.identifier = "TestIdentifier"
	resultValue = readAndExecute(&testReceive, &config)
	if resultValue.source != "Mod-Gearman Worker @ TestIdentifier" {
		t.Errorf("got %s but expected: %s", resultValue.source, "Mod-Gearman Worker @ TestIdentifier")
	}

	// check for max age
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

	executeCommand(result, &receivedStruct{commandLine: "ls readAndExecute_test.go", timeout: 10}, &config)
	if result.output != "readAndExecute_test.go" || result.returnCode != 0 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "readAndExecute_test.go", 0)
	}

	// check for timeout:
	t1 := time.Now()
	config.timeoutReturn = 3
	executeCommand(result, &receivedStruct{commandLine: "/bin/sleep 2", timeout: 1}, &config)
	if !strings.HasPrefix(result.output, "(Check Timed Out On Worker:") || result.returnCode != 3 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "timeout", 3)
	}
	duration := time.Since(t1)
	if duration > 2*time.Second {
		t.Errorf("command took %s, which is beyond the expected timeout", duration)
	}

	// try command which ignores normal signals
	t1 = time.Now()
	executeCommand(result, &receivedStruct{commandLine: "trap 'echo Booh!' SIGINT SIGTERM; sleep 2", timeout: 1}, &config)
	if !strings.HasPrefix(result.output, "(Check Timed Out On Worker:") || result.returnCode != 3 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "timeout", 3)
	}
	duration = time.Since(t1)
	if duration > 2*time.Second {
		t.Errorf("command took %s, which is beyond the expected timeout", duration)
	}

	// try command which ignores normal signals as subshell
	t1 = time.Now()
	executeCommand(result, &receivedStruct{commandLine: "/bin/sh -c \"trap 'echo Booh!' SIGINT SIGTERM; sleep 2\"", timeout: 1}, &config)
	if !strings.HasPrefix(result.output, "(Check Timed Out On Worker:") || result.returnCode != 3 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "timeout", 3)
	}
	duration = time.Since(t1)
	if duration > 2*time.Second {
		t.Errorf("command took %s, which is beyond the expected timeout", duration)
	}

	// exit(3) fuer exit codes
	executeCommand(result, &receivedStruct{commandLine: "/bin/sh -c \"echo 'exit status 2'; exit 2\"", timeout: 10}, &config)
	if result.output != "exit status 2" || result.returnCode != 2 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "exit status 2", 2)
	}

	// stdout & stderr output
	executeCommand(result, &receivedStruct{commandLine: "/bin/sh -c \"echo 'stderr\nstderr' >&2; echo 'stdout\nstdout'; exit 2\"", timeout: 10}, &config)
	if result.output != `stdout\nstdout\n[stderr\nstderr]` || result.returnCode != 2 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, `stdout\nstdout\n[stderr\nstderr]`, 2)
	}

	// stderr output only
	executeCommand(result, &receivedStruct{commandLine: "/bin/sh -c \"echo 'stderr' >&2; exit 2\"", timeout: 10}, &config)
	if result.output != "[stderr]" || result.returnCode != 2 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "[stderr]", 2)
	}

	// quotes in output
	executeCommand(result, &receivedStruct{commandLine: "/bin/sh -c \"echo '\\\"'\"", timeout: 10}, &config)
	if result.output != "\"" || result.returnCode != 0 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "\"", 0)
	}

	// none-existing command
	executeCommand(result, &receivedStruct{commandLine: "/not-there", timeout: 3}, &config)
	if !strings.HasPrefix(result.output, "UNKNOWN: Return code of 127 is out of bounds.") || result.returnCode != 3 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "UNKNOWN: Return code of 127 is out of bounds.", 3)
	}

	// none-existing command II
	executeCommand(result, &receivedStruct{commandLine: "/not-there \"\"", timeout: 3}, &config)
	if !strings.HasPrefix(result.output, "CRITICAL: Return code of 127 is out of bounds.") || result.returnCode != 2 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "CRITICAL: Return code of 127 is out of bounds.", 2)
	}

	// other exit codes
	executeCommand(result, &receivedStruct{commandLine: "/bin/sh -c \"echo 'exit status 42'; exit 42\"", timeout: 10}, &config)
	if !strings.HasPrefix(result.output, "CRITICAL: Return code of 42 is out of bounds.") || result.returnCode != 3 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "CRITICAL: Return code of 42 is out of bounds.", 3)
	}

	// signals
	executeCommand(result, &receivedStruct{commandLine: "/bin/sh -c \"echo 'killing\nme...'; echo 'stderr\nstderr' >&2; kill $$\"", timeout: 10}, &config)
	if !strings.HasPrefix(result.output, "CRITICAL: Return code of 15 is out of bounds.") || result.returnCode != 2 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "CRITICAL: Return code of 15 is out of bounds.", 2)
	}
	if !strings.Contains(result.output, `)\nkilling\nme...\n[stderr\nstderr]`) || result.returnCode != 2 {
		t.Errorf("got result %s, but expected containing %s", result.output, `)\nkilling\nme...\n[stderr\nstderr]`)
	}
}

func TestExecuteCommandArgListTooLongError(t *testing.T) {
	config := configurationStruct{}
	setDefaultValues(&config)
	config.encryption = false
	result := &answer{}

	executeCommand(result, &receivedStruct{commandLine: "getconf ARG_MAX", timeout: 10}, &config)
	argMax, err := strconv.ParseInt(result.output, 0, 64)
	if err != nil || argMax <= 0 {
		t.Skip("skipping test without ARG_MAX")
	}

	if argMax > math.MaxInt32 {
		t.Skip("skipping test integer too small")
	}

	// create a cmd which should trigger arguments too long error
	cmd := "/bin/sh -c echo " + string(bytes.Repeat([]byte{1}, int(argMax-1)))
	executeCommand(result, &receivedStruct{commandLine: cmd, timeout: 10}, &config)
	if !strings.Contains(result.output, `argument list too long`) || result.returnCode != 3 {
		t.Errorf("got result %s, but expected containing %s", result.output, `argument list too long`)
	}
}

func TestExecuteCommandOutOfFilesError(t *testing.T) {
	if os.Getenv("PANIC_TESTS") == "" {
		t.Skip("test will panic, run manually with PANIC_TESTS=1 env set")
	}
	config := configurationStruct{}
	setDefaultValues(&config)
	config.encryption = false
	result := &answer{}

	var rLimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		t.Skip("cannot get current rlimit")
	}
	rLimit.Cur = 10
	err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		t.Skip("cannot set rlimit")
	}

	// create a cmd which should trigger out of files error
	executeCommand(result, &receivedStruct{commandLine: "/bin/true", timeout: 10}, &config)
	if !strings.Contains(result.output, `too many open files`) || result.returnCode != 3 {
		t.Errorf("got result %s, but expected containing %s", result.output, `too many open files`)
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

func BenchmarkExecuteCommandWithTimeout(b *testing.B) {
	config := configurationStruct{}
	config.debug = 0
	createLogger(&config)
	for n := 0; n < b.N; n++ {
		executeCommand(&answer{}, &receivedStruct{commandLine: "/bin/pwd", timeout: 100}, &config)
	}
}

func BenchmarkReadAndExecuteShell(b *testing.B) {
	config := configurationStruct{}
	config.debug = 0
	createLogger(&config)
	received := &receivedStruct{commandLine: "/bin/pwd \"\"", timeout: 10}
	for n := 0; n < b.N; n++ {
		readAndExecute(received, &config)
	}
}

func BenchmarkReadAndExecuteExec(b *testing.B) {
	config := configurationStruct{}
	config.debug = 0
	createLogger(&config)
	received := &receivedStruct{commandLine: "/bin/pwd", timeout: 10}
	for n := 0; n < b.N; n++ {
		readAndExecute(received, &config)
	}
}
