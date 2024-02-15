package modgearman

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommandParser_1(t *testing.T) {
	cmdLine := "/bin/command"
	cmd := parseCommand(cmdLine, &configurationStruct{})
	assert.Equal(t, Exec, cmd.ExecType)
	assert.Equal(t, "/bin/command", cmd.Command)
	assert.Equal(t, []string{}, cmd.Args)
	assert.Equal(t, map[string]string{}, cmd.Env)
}

func TestCommandParser_2(t *testing.T) {
	cmdLine := "/bin/command args1"
	cmd := parseCommand(cmdLine, &configurationStruct{})
	assert.Equal(t, Exec, cmd.ExecType)
	assert.Equal(t, "/bin/command", cmd.Command)
	assert.Equal(t, []string{"args1"}, cmd.Args)
	assert.Equal(t, map[string]string{}, cmd.Env)
}

func TestCommandParser_3(t *testing.T) {
	cmdLine := `ENV1='var' ./bin/command "args 123" "TEST"`
	cmd := parseCommand(cmdLine, &configurationStruct{})
	assert.Equal(t, Exec, cmd.ExecType)
	assert.Equal(t, "./bin/command", cmd.Command)
	assert.Equal(t, []string{"args 123", "TEST"}, cmd.Args)
	assert.Equal(t, map[string]string{"ENV1": "var"}, cmd.Env)
}

func TestCommandParser_3b(t *testing.T) {
	cmdLine := `ENV1='var space' ./bin/command "args 123" "TEST"`
	cmd := parseCommand(cmdLine, &configurationStruct{})
	assert.Equal(t, Exec, cmd.ExecType)
	assert.Equal(t, "./bin/command", cmd.Command)
	assert.Equal(t, []string{"args 123", "TEST"}, cmd.Args)
	assert.Equal(t, map[string]string{"ENV1": "var space"}, cmd.Env)
}

// unparsable trailing quotes -> shell
func TestCommandParser_4(t *testing.T) {
	cmdLine := `ENV1='var' ./bin/command "args 123" "TEST`
	cmd := parseCommand(cmdLine, &configurationStruct{})
	assert.Equal(t, Shell, cmd.ExecType)
	assert.Equal(t, "/bin/sh", cmd.Command)
	assert.Equal(t, []string{"-c", cmdLine}, cmd.Args)
	assert.Equal(t, map[string]string{}, cmd.Env)
}

func TestCommandParser_5(t *testing.T) {
	cmdLine := `trap 'echo Booh!' SIGINT SIGTERM; sleep 2`
	cmd := parseCommand(cmdLine, &configurationStruct{})
	assert.Equal(t, Shell, cmd.ExecType)
	assert.Equal(t, "/bin/sh", cmd.Command)
	assert.Equal(t, []string{"-c", cmdLine}, cmd.Args)
	assert.Equal(t, map[string]string{}, cmd.Env)
}

func TestCommandParser_BackslashDouble(t *testing.T) {
	cmdLine := `test.sh "\t\n"`
	cmd := parseCommand(cmdLine, &configurationStruct{})
	assert.Equal(t, Exec, cmd.ExecType)
	assert.Equal(t, "test.sh", cmd.Command)
	assert.Equal(t, []string{`\t\n`}, cmd.Args)
	assert.Equal(t, map[string]string{}, cmd.Env)
}

func TestCommandParser_BackslashSingle(t *testing.T) {
	cmdLine := `test.sh '\t\n'`
	cmd := parseCommand(cmdLine, &configurationStruct{})
	assert.Equal(t, Exec, cmd.ExecType)
	assert.Equal(t, "test.sh", cmd.Command)
	assert.Equal(t, []string{`\t\n`}, cmd.Args)
	assert.Equal(t, map[string]string{}, cmd.Env)
}
