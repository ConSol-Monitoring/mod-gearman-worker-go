package modgearman

import (
	"testing"
)

func TestCommandParser_1(t *testing.T) {
	cmdLine := "/bin/command"
	cmd := parseCommand(cmdLine, &configurationStruct{})
	if cmd.ExecType != Exec {
		t.Errorf("exec type differs: %v vs. %v", cmd.ExecType, Exec)
	}
	if cmd.Command != cmdLine {
		t.Errorf("command differs: %v vs. %v", cmd.Command, cmdLine)
	}
	if len(cmd.Args) != 0 {
		t.Errorf("command args wrong")
	}
	if len(cmd.Env) != 0 {
		t.Errorf("command env wrong")
	}
}

func TestCommandParser_2(t *testing.T) {
	cmdLine := "/bin/command args1"
	cmd := parseCommand(cmdLine, &configurationStruct{})
	if cmd.ExecType != Exec {
		t.Errorf("exec type differs: %v vs. %v", cmd.ExecType, Exec)
	}
	if cmd.Command != "/bin/command" {
		t.Errorf("command differs: %v vs. %v", cmd.Command, cmdLine)
	}
	if len(cmd.Args) != 1 {
		t.Errorf("command args wrong")
	}
	if cmd.Args[0] != "args1" {
		t.Errorf("command args differs: %v", cmd.Args)
	}
	if len(cmd.Env) != 0 {
		t.Errorf("command env wrong")
	}
}

func TestCommandParser_3(t *testing.T) {
	cmdLine := `ENV1='var' ./bin/command "args 123" "TEST"`
	cmd := parseCommand(cmdLine, &configurationStruct{})
	if cmd.ExecType != Exec {
		t.Errorf("exec type differs: %v vs. %v", cmd.ExecType, Shell)
	}
	if cmd.Command != "./bin/command" {
		t.Errorf("command differs: %v vs. %v", cmd.Command, cmdLine)
	}
	if len(cmd.Args) != 2 {
		t.Errorf("command args wrong")
	}
	if cmd.Args[0] != "args 123" {
		t.Errorf("command args differs: %v", cmd.Args)
	}
	if len(cmd.Env) != 1 {
		t.Errorf("command env wrong")
	}
	if cmd.Env["ENV1"] != "var" {
		t.Errorf("command env differs: %v", cmd.Env)
	}
}

// unparsable trailing quotes -> shell
func TestCommandParser_4(t *testing.T) {
	cmdLine := `ENV1='var' ./bin/command "args 123" "TEST`
	cmd := parseCommand(cmdLine, &configurationStruct{})
	if cmd.ExecType != Shell {
		t.Errorf("exec type differs: %v vs. %v", cmd.ExecType, Shell)
	}
	if cmd.Command != cmdLine {
		t.Errorf("command differs: %v vs. %v", cmd.Command, cmdLine)
	}
	if len(cmd.Args) != 0 {
		t.Errorf("command args wrong")
	}
	if len(cmd.Env) != 0 {
		t.Errorf("command env wrong")
	}
}

func TestCommandParser_5(t *testing.T) {
	cmdLine := `trap 'echo Booh!' SIGINT SIGTERM; sleep 2`
	cmd := parseCommand(cmdLine, &configurationStruct{})
	if cmd.ExecType != Shell {
		t.Errorf("exec type differs: %v vs. %v", cmd.ExecType, Shell)
	}
	if cmd.Command != cmdLine {
		t.Errorf("command differs: %v vs. %v", cmd.Command, cmdLine)
	}
	if len(cmd.Args) != 0 {
		t.Errorf("command args wrong")
	}
	if len(cmd.Env) != 0 {
		t.Errorf("command env wrong")
	}
}
