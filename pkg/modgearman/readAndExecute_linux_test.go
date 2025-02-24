package modgearman

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestReadAndExecute(t *testing.T) {
	cfg := config{}
	cfg.encryption = false
	cfg.jobTimeout = 30
	cfg.setDefaultValues()
	thisHostname, err := os.Hostname()
	if err != nil {
		thisHostname = "unknown"
	}
	// create the test received only with needed values
	var testReceive request
	// result_queue, core_start_time, hostname should always stay the same
	testReceive.resultQueue = "TestQueue"
	testReceive.coreTime = 123
	testReceive.hostName = "TestHost"
	testReceive.serviceDescription = "TestDescription"
	testReceive.commandLine = "sleep 1"

	resultValue := readAndExecute(&testReceive, &cfg)
	expSrc := "Mod-Gearman Worker @ " + thisHostname

	if resultValue.source != expSrc || resultValue.resultQueue != "TestQueue" || resultValue.coreStartTime != 123 ||
		resultValue.hostName != "TestHost" || resultValue.serviceDescription != "TestDescription" {
		t.Errorf("got %s but expected: %s",
			resultValue, expSrc+"result_queue = TestQueue, core_start_time = 123, service_description = TestDescription")
	}

	// same but with identifier in config file
	cfg.identifier = "TestIdentifier"
	resultValue = readAndExecute(&testReceive, &cfg)
	if resultValue.source != "Mod-Gearman Worker @ TestIdentifier" {
		t.Errorf("got %s but expected: %s", resultValue.source, "Mod-Gearman Worker @ TestIdentifier")
	}

	// check for max age
	cfg.maxAge = 1
	testReceive.coreTime = float64(time.Now().UnixNano())/1e9 - 2
	resultValue = readAndExecute(&testReceive, &cfg)
	expectedOutput := fmt.Sprintf("Could not start check in time (worker: %s)", cfg.identifier)
	if resultValue.output != expectedOutput {
		t.Errorf("got %s but expected: %s", resultValue.output, expectedOutput)
	}
}

func TestExecuteCommandWithTimeout(t *testing.T) {
	cfg := config{}
	cfg.setDefaultValues()
	cfg.encryption = false
	result := &answer{}

	executeCommandLine(result, &request{commandLine: "ls readAndExecute_linux_test.go", timeout: 10}, &cfg)
	if result.output != "readAndExecute_linux_test.go" || result.returnCode != 0 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d",
			result.output, result.returnCode, "readAndExecute_test.go", 0)
	}

	// check for timeout:
	time1 := time.Now()
	cfg.timeoutReturn = 3
	result = &answer{}
	executeCommandLine(result, &request{commandLine: "/bin/sleep 2", timeout: 1}, &cfg)
	if !strings.HasPrefix(result.output, "(Check Timed Out On Worker:") || result.returnCode != 3 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "timeout", 3)
	}
	duration := time.Since(time1)
	if duration > 2*time.Second {
		t.Errorf("command took %s, which is beyond the expected timeout", duration)
	}

	// try command which ignores normal signals
	time1 = time.Now()
	result = &answer{}
	executeCommandLine(result, &request{
		commandLine: "trap 'echo Booh!' SIGINT SIGTERM; sleep 2",
		timeout:     1,
	}, &cfg)
	if !strings.HasPrefix(result.output, "(Check Timed Out On Worker:") || result.returnCode != 3 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "timeout", 3)
	}
	duration = time.Since(time1)
	if duration > 2*time.Second {
		t.Errorf("command took %s, which is beyond the expected timeout", duration)
	}

	// try command which ignores normal signals as subshell
	time1 = time.Now()
	result = &answer{}
	executeCommandLine(result, &request{
		commandLine: "/bin/sh -c \"trap 'echo Booh!' SIGINT SIGTERM; sleep 2\"",
		timeout:     1,
	}, &cfg)
	if !strings.HasPrefix(result.output, "(Check Timed Out On Worker:") || result.returnCode != 3 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "timeout", 3)
	}
	duration = time.Since(time1)
	if duration > 2*time.Second {
		t.Errorf("command took %s, which is beyond the expected timeout", duration)
	}
	// exit(3) fuer exit codes
	result = &answer{}
	executeCommandLine(result, &request{
		commandLine: "/bin/sh -c \"echo 'exit status 2'; exit 2\"",
		timeout:     10,
	}, &cfg)
	if result.output != "exit status 2" || result.returnCode != 2 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "exit status 2", 2)
	}

	// stdout & stderr output
	result = &answer{}
	executeCommandLine(result, &request{
		commandLine: "/bin/sh -c \"echo 'stderr\nstderr' >&2; echo 'stdout\nstdout'; exit 2\"",
		timeout:     10,
	}, &cfg)
	if result.output != `stdout\nstdout\n[stderr\nstderr]` || result.returnCode != 2 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d",
			result.output, result.returnCode, `stdout\nstdout\n[stderr\nstderr]`, 2)
	}

	// stderr output only
	result = &answer{}
	executeCommandLine(result, &request{
		commandLine: "/bin/sh -c \"echo 'stderr' >&2; exit 2\"",
		timeout:     10,
	}, &cfg)
	if result.output != "[stderr]" || result.returnCode != 2 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "[stderr]", 2)
	}

	// quotes in output
	result = &answer{}
	executeCommandLine(result, &request{commandLine: "/bin/sh -c \"echo '\\\"'\"", timeout: 10}, &cfg)
	if result.output != "\"" || result.returnCode != 0 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d", result.output, result.returnCode, "\"", 0)
	}

	// none-existing command
	result = &answer{}
	executeCommandLine(result, &request{commandLine: "/not-there", timeout: 3}, &cfg)
	if !strings.HasPrefix(result.output, "UNKNOWN: Return code of 127 is out of bounds.") || result.returnCode != 3 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d",
			result.output, result.returnCode, "UNKNOWN: Return code of 127 is out of bounds.", 3)
	}

	// none-existing command II
	result = &answer{}
	executeCommandLine(result, &request{commandLine: "/not-there \"\"", timeout: 3}, &cfg)
	if !strings.HasPrefix(result.output, "UNKNOWN: Return code of 127 is out of bounds.") || result.returnCode != 3 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d",
			result.output, result.returnCode, "UNKNOWN: Return code of 127 is out of bounds.", 3)
	}

	// other exit codes
	result = &answer{}
	executeCommandLine(result, &request{
		commandLine: "/bin/sh -c \"echo 'exit status 42'; exit 42\"",
		timeout:     10,
	}, &cfg)
	if !strings.HasPrefix(result.output, "CRITICAL: Return code of 42 is out of bounds.") || result.returnCode != 3 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d",
			result.output, result.returnCode, "CRITICAL: Return code of 42 is out of bounds.", 3)
	}

	// signals
	result = &answer{}
	executeCommandLine(result, &request{
		commandLine: "/bin/sh -c \"echo 'killing\nme...'; echo 'stderr\nstderr' >&2; kill $$\"",
		timeout:     10,
	}, &cfg)
	if !strings.HasPrefix(result.output, "UNKNOWN: Return code of 15 is out of bounds.") || result.returnCode != 3 {
		t.Errorf("got %s, with code: %d but expected: %s and code: %d",
			result.output, result.returnCode, "UNKNOWN: Return code of 15 is out of bounds.", 3)
	}

	if !strings.Contains(result.output, `)\nkilling\nme...\n[stderr\nstderr]`) || result.returnCode != 3 {
		t.Errorf("got result %s, but expected containing %s", result.output, `)\nkilling\nme...\n[stderr\nstderr]`)
	}
}

func TestExecuteCommandArgListTooLongError(t *testing.T) {
	cfg := config{}
	cfg.setDefaultValues()
	cfg.encryption = false
	result := &answer{}

	executeCommandLine(result, &request{commandLine: "getconf ARG_MAX", timeout: 10}, &cfg)
	argMax, err := strconv.ParseInt(result.output, 0, 64)
	if err != nil || argMax <= 0 {
		t.Skip("skipping test without ARG_MAX")
	}

	if argMax > math.MaxInt32 {
		t.Skip("skipping test integer too small")
	}

	// create a cmd which should trigger arguments too long error
	cmd := "/bin/sh -c echo " + string(bytes.Repeat([]byte{1}, int(argMax-1)))
	executeCommandLine(result, &request{commandLine: cmd, timeout: 10}, &cfg)
	if !strings.Contains(result.output, `argument list too long`) || result.returnCode != 3 {
		t.Errorf("got result %s, but expected containing %s", result.output, `argument list too long`)
	}
}

func TestExecuteCommandOutOfFilesError(t *testing.T) {
	if os.Getenv("PANIC_TESTS") == "" {
		t.Skip("test will panic, run manually with PANIC_TESTS=1 env set")
	}
	cfg := config{}
	cfg.setDefaultValues()
	cfg.encryption = false
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
	executeCommandLine(result, &request{commandLine: "/bin/true", timeout: 10}, &cfg)
	if !strings.Contains(result.output, `too many open files`) || result.returnCode != 3 {
		t.Errorf("got result %s, but expected containing %s", result.output, `too many open files`)
	}
}

func TestExecuteCommandFunnyQuotes(t *testing.T) {
	cfg := config{}
	cfg.setDefaultValues()
	cfg.encryption = false
	result := &answer{}

	// create a cmd which should trigger out of files error
	executeCommandLine(result, &request{
		commandLine: `echo a '''b''' '' """c""" ''d'' ee""ee f' 'f '" "' "' ''"`,
		timeout:     10,
	}, &cfg)
	assert.Equal(t, "exec", result.execType)
	assert.Equal(t, `a b  c d eeee f f " " ' ''`, result.output, "output from funny quoted argument")

	expect := `$VAR1 = [\n          'a',\n          'b',\n          '',\n          'c',\n          'd',\n          'eeee',\n          'f f',\n          '" "',\n          '\' \'\''\n        ];`
	executeCommandLine(result, &request{
		commandLine: `perl -MData::Dumper -e 'print Dumper \@ARGV' -- a """b""" '' ''c'' ''d'' ee""ee f' 'f '" "' "' ''"`,
		timeout:     10,
	}, &cfg)
	assert.Equal(t, expect, result.output, "output from funny quoted argument")
	assert.Equal(t, "exec", result.execType)
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
		{"/python /tmp/file1 args1", "python file1"},
		{"/python2 /tmp/file1 args1", "python2 file1"},
		{"/python3 /tmp/file1 args1", "python3 file1"},
		{"lib/negate /bin/python3 /tmp/file1 args1", "python3 file1"},
		{`ENV1="1 2 3" ENV2='2' ./test arg1 -P 'm1|m2';`, "test"},
		{"/python3 -m testmodule args", "python3 testmodule"},
		{"/python3 -u somescript args", "python3 somescript"},
	}

	for _, test := range tests {
		com := parseCommand(test.command, &config{internalNegate: true})
		base := getCommandQualifier(com)
		assert.Equal(t, test.expect, base, "getCommandQualifier")
	}
}

func BenchmarkReadAndExecuteShell(b *testing.B) {
	cfg := config{}
	cfg.debug = 0
	createLogger(&cfg)
	received := &request{commandLine: "/bin/pwd \"|\"", timeout: 10}
	for range b.N {
		readAndExecute(received, &cfg)
	}
}

func BenchmarkReadAndExecuteExec(b *testing.B) {
	cfg := config{}
	cfg.debug = 0
	createLogger(&cfg)
	received := &request{commandLine: "/bin/pwd", timeout: 10}
	for range b.N {
		readAndExecute(received, &cfg)
	}
}

func BenchmarkParseCommandI(b *testing.B) {
	cfg := config{}
	cfg.debug = 0
	cmdLine := `VAR1=test VAR2=test /bin/test -f "sadhajshdkashdjhasjkdhjashdkhasdhakshdashdkhaskjdhaksjhdkjahsdkjhaskjdhasjhdkashdkjhaskjdhaksjhdkjahsdkjhaskjdhakshdkashkd hjskahd ash sadhajshdkashdjhasjkdhjashdkhasdhakshdashdkhaskjdhaksjhdkjahsdkjhaskjdhasjhdkashdkjhaskjdhaksjhdkjahsdkjhaskjdhakshdkashkd  sadhajshdkashdjhasjkdhjashdkhasdhakshdashdkhaskjdhaksjhdkjahsdkjhaskjdhasjhdkashdkjhaskjdhaksjhdkjahsdkjhaskjdhakshdkashkd sadhajshdkashdjhasjkdhjashdkhasdhakshdashdkhaskjdhaksjhdkjahsdkjhaskjdhasjhdkashdkjhaskjdhaksjhdkjahsdkjhaskjdhakshdkashkd  da s a dasdjhaskdhkashdkjhaskjhdkjas  ashdjahskdjhakshdkjahskd   ashdkjahsdkhaskdhkjashd"`
	for range b.N {
		parseCommand(cmdLine, &cfg)
	}
	com := parseCommand(cmdLine, &cfg)
	assert.Equal(b, "/bin/test", com.Command, "command parsed")
	assert.Equalf(b, map[string]string{"VAR1": "test", "VAR2": "test"}, com.Env, "env parsed")
	assert.Lenf(b, com.Args, 2, "args parsed")
}

func BenchmarkParseCommandII(b *testing.B) {
	cfg := config{}
	cfg.debug = 0
	cmdLine := `VAR1=test VAR2=test /bin/test -f sadhajshdkashdjhasjkdhjashdkhasdhakshdashdkhaskjdhaksjhdkjahsdkjhaskjdhasjhdkashdkjhaskjdhaksjhdkjahsdkjhaskjdhakshdkashkd hjskahd ash sadhajshdkashdjhasjkdhjashdkhasdhakshdashdkhaskjdhaksjhdkjahsdkjhaskjdhasjhdkashdkjhaskjdhaksjhdkjahsdkjhaskjdhakshdkashkd  sadhajshdkashdjhasjkdhjashdkhasdhakshdashdkhaskjdhaksjhdkjahsdkjhaskjdhasjhdkashdkjhaskjdhaksjhdkjahsdkjhaskjdhakshdkashkd sadhajshdkashdjhasjkdhjashdkhasdhakshdashdkhaskjdhaksjhdkjahsdkjhaskjdhasjhdkashdkjhaskjdhaksjhdkjahsdkjhaskjdhakshdkashkd  da s a dasdjhaskdhkashdkjhaskjhdkjas  ashdjahskdjhakshdkjahskd   ashdkjahsdkhaskdhkjashd`
	for range b.N {
		parseCommand(cmdLine, &cfg)
	}
	com := parseCommand(cmdLine, &cfg)
	assert.Equal(b, "/bin/test", com.Command, "command parsed")
	assert.Equalf(b, map[string]string{"VAR1": "test", "VAR2": "test"}, com.Env, "env parsed")
	assert.Lenf(b, com.Args, 13, "args parsed")
}
