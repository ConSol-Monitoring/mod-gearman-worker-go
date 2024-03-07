package modgearman

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDummy(t *testing.T) {
	cmdLine := `ENV1=test ./lib/check_dummy 0 'test output'`
	cmd := parseCommand(cmdLine, &config{internalCheckDummy: true})

	assert.Equal(t, Internal, cmd.ExecType, "exec type")
	assert.Equal(t, &InternalCheckDummy{}, cmd.InternalCheck, "exec type")
}

func TestDummyExecute(t *testing.T) {
	cfg := config{}
	cfg.setDefaultValues()
	cfg.encryption = false
	result := &answer{}

	executeCommandLine(result, &request{
		commandLine: `ENV1=test ./lib/check_dummy 0 'test output'`,
		timeout:     10,
	}, &cfg)
	assert.Equal(t, "internal", result.execType, "exec type")
	assert.Equal(t, 0, result.returnCode, "return code")
	assert.Equal(t, "OK: test output", result.output, "output")
}

func TestDummyExecuteWarning(t *testing.T) {
	cfg := config{}
	cfg.setDefaultValues()
	cfg.encryption = false
	result := &answer{}

	executeCommandLine(result, &request{
		commandLine: `ENV1=test ./lib/check_dummy 1 test' 'out"put"`,
		timeout:     10,
	}, &cfg)
	assert.Equal(t, "internal", result.execType, "exec type")
	assert.Equal(t, 1, result.returnCode, "return code")
	assert.Equal(t, "WARNING: test output", result.output, "output")
}
