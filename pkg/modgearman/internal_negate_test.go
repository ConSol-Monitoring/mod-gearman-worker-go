package modgearman

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNegate(t *testing.T) {
	cmdLine := "ENV1=test ./lib/negate -w OK -c UNKNOWN -s /bin/command comArg1 comArg2"
	cmd := parseCommand(cmdLine, &configurationStruct{internalNegate: true})

	assert.Equal(t, Exec, cmd.ExecType, "exec type")
	assert.NotNil(t, cmd.Negate, "parsed negate")
	assert.Equal(t, "OK", cmd.Negate.WarningStatus, "parsed negate")
	assert.Equal(t, "UNKNOWN", cmd.Negate.CriticalStatus, "parsed negate")
}

func TestExecuteCommandWithNegateI(t *testing.T) {
	config := configurationStruct{}
	config.setDefaultValues()
	config.encryption = false
	result := &answer{}

	executeCommandLine(result, &receivedStruct{commandLine: "lib/negate -o 1 -s /bin/sh -c 'echo \"OK - failed\"'", timeout: 10}, &config)
	assert.Equal(t, "exec", result.execType, "exec type")
	assert.Equal(t, 1, result.returnCode, "return code")
	assert.Equal(t, "WARNING - failed", result.output, "output replaced")
}

func TestExecuteCommandWithNegateNoOptions(t *testing.T) {
	config := configurationStruct{}
	config.setDefaultValues()
	config.encryption = false
	result := &answer{}

	executeCommandLine(result, &receivedStruct{commandLine: "lib/negate -s /bin/sh -c 'echo \"OK - fine\"'", timeout: 10}, &config)
	assert.Equal(t, "exec", result.execType, "exec type")
	assert.Equal(t, 2, result.returnCode, "return code")
	assert.Equal(t, "CRITICAL - fine", result.output, "output replaced")
}

func TestExecuteCommandWithNegateTimeout(t *testing.T) {
	config := configurationStruct{}
	config.setDefaultValues()
	config.encryption = false
	result := &answer{}

	executeCommandLine(result, &receivedStruct{commandLine: "lib/negate -t 1 -s /bin/sh -c 'sleep 3'", timeout: 10}, &config)
	assert.Equal(t, "exec", result.execType, "exec type")
	assert.Equal(t, 3, result.returnCode, "return code")
	assert.Contains(t, result.output, "Check Timed Out On", "timeout output")
}
