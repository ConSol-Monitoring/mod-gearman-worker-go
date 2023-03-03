package modgearman

import (
	"strings"

	shellwords "github.com/mattn/go-shellwords"
)

// CommandExecType is used to set the execution path
type CommandExecType int

const (
	// Shell uses /bin/sh
	Shell CommandExecType = iota

	// Exec uses exec without a shell
	Exec

	// EPN is the embedded perl interpreter
	EPN
)

type command struct {
	ExecType CommandExecType
	Command  string
	Args     []string
	Env      map[string]string
}

func parseCommand(rawCommand string, config *configurationStruct) *command {
	parsed := &command{
		ExecType: Shell,
		Command:  rawCommand,
		Args:     make([]string, 0),
		Env:      make(map[string]string),
	}

	// don't try to parse super long command lines
	if len(rawCommand) > 10000 {
		return parsed
	}

	if strings.ContainsAny(rawCommand, "!$^&*()~[]\\|{};<>?`") {
		return parsed
	}

	envs, args, err := shellwords.ParseWithEnvs(rawCommand)
	if err != nil {
		logger.Debugf("failed to parse shell words: %w: %s", err)
		return parsed
	}
	parsed.Command = args[0]
	parsed.Args = args[1:]
	parsed.ExecType = Exec
	for _, env := range envs {
		splitted := strings.SplitN(env, "=", 2)
		parsed.Env[splitted[0]] = splitted[1]
	}

	if fileUsesEmbeddedPerl(parsed.Command, config) {
		parsed.ExecType = EPN
	}

	return parsed
}
