package modgearman

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNegateParse(t *testing.T) {
	cmdLine := "ENV1=test ./lib/negate -w OK -c UNKNOWN -s /bin/command comArg1 comArg2"
	cmd := parseCommand(cmdLine, &config{internalNegate: true})

	assert.Equal(t, Exec, cmd.ExecType, "exec type")
	assert.NotNil(t, cmd.Negate, "parsed negate")
	assert.Equal(t, map[string]string{"ENV1": "test"}, cmd.Env)
	assert.Equal(t, "OK", cmd.Negate.WarningStatus, "parsed negate")
	assert.Equal(t, "UNKNOWN", cmd.Negate.CriticalStatus, "parsed negate")
}

func TestNegateParseQuoted(t *testing.T) {
	cmdLine := "./lib/negate \"/bin/bash comArg1 comArg2 ABC=123 -c 'te st'\""
	cmd := parseCommand(cmdLine, &config{internalNegate: true})

	assert.Equal(t, Exec, cmd.ExecType, "exec type")
	assert.NotNil(t, cmd.Negate, "parsed negate")
	assert.Equal(t, "/bin/bash", cmd.Command, "remaining parsed command")
	assert.Equal(t, []string{"comArg1", "comArg2", "ABC=123", "-c", "te st"}, cmd.Args, "remaining args")
}

func TestNegateParseQuotedShell(t *testing.T) {
	cmdLine := "lib/negate -t 1 -s /bin/sh -c 'sleep 3'"
	cmd := parseCommand(cmdLine, &config{internalNegate: true})

	assert.Equal(t, Exec, cmd.ExecType, "exec type")
	assert.NotNil(t, cmd.Negate, "parsed negate")
	assert.Equal(t, "/bin/sh", cmd.Command, "remaining parsed command")
	assert.Equal(t, []string{"-c", "sleep 3"}, cmd.Args, "remaining args")
}

func TestNegateParseQuotedShellChars(t *testing.T) {
	cmdLine := "lib/negate -t 1 -s /bin/sh -c 'sleep 3; echo $SHELL'"
	cmd := parseCommand(cmdLine, &config{internalNegate: true})

	assert.Equal(t, Exec, cmd.ExecType, "exec type")
	assert.NotNil(t, cmd.Negate, "parsed negate")
	assert.Equal(t, "/bin/sh", cmd.Command, "remaining parsed command")
	assert.Equal(t, []string{"-c", "sleep 3; echo $SHELL"}, cmd.Args, "remaining args")
}

func TestExecuteCommandWithNegateI(t *testing.T) {
	cfg := config{}
	cfg.setDefaultValues()
	cfg.encryption = false
	result := &answer{}

	executeCommandLine(result, &request{
		commandLine: "lib/negate -o 1 -s /bin/sh -c 'echo \"OK - failed\"'",
		timeout:     10,
	}, &cfg)
	assert.Equal(t, "exec", result.execType, "exec type")
	assert.Equal(t, 1, result.returnCode, "return code")
	assert.Equal(t, "WARNING - failed", result.output, "output replaced")
}

func TestExecuteCommandWithNegateII(t *testing.T) {
	cfg := config{}
	cfg.setDefaultValues()
	cfg.encryption = false
	result := &answer{}

	executeCommandLine(result, &request{
		commandLine: "lib/negate -o 1 \"/bin/sh -c 'echo OK'\"",
		timeout:     10,
	}, &cfg)
	assert.Equal(t, "exec", result.execType, "exec type")
	assert.Equal(t, 1, result.returnCode, "return code")
	assert.Equal(t, "OK", result.output, "output replaced")
}

func TestExecuteCommandWithNegateNoOptions(t *testing.T) {
	cfg := config{}
	cfg.setDefaultValues()
	cfg.encryption = false
	result := &answer{}

	executeCommandLine(result, &request{
		commandLine: "lib/negate -s /bin/sh -c 'echo \"OK - fine\"'",
		timeout:     10,
	}, &cfg)
	assert.Equal(t, "exec", result.execType, "exec type")
	assert.Equal(t, 2, result.returnCode, "return code")
	assert.Equal(t, "CRITICAL - fine", result.output, "output replaced")
}

func TestExecuteCommandWithNegateTimeout(t *testing.T) {
	cfg := config{}
	cfg.setDefaultValues()
	cfg.encryption = false
	result := &answer{}

	executeCommandLine(result, &request{
		commandLine: "lib/negate -t 1 -s /bin/sh -c 'sleep 3'",
		timeout:     10,
	}, &cfg)
	assert.Equal(t, "exec", result.execType, "exec type")
	assert.Equal(t, 3, result.returnCode, "return code")
	assert.Contains(t, result.output, "Check Timed Out On", "timeout output")
}
