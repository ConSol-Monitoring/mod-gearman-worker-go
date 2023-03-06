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
	Negate   *Negate
}

func parseCommand(rawCommand string, config *configurationStruct) *command {
	parsed := &command{
		ExecType: Shell,
		Command:  rawCommand,
		Args:     make([]string, 0),
		Env:      make(map[string]string),
	}

	// adjust command to run with a shell
	defer func() {
		if parsed.ExecType == Shell {
			parsed.Args = []string{"-c", parsed.Command}
			parsed.Command = "/bin/sh"
		}
	}()

	if strings.ContainsAny(rawCommand, "!$^&*()~[]\\|{};<>?`") {
		return parsed
	}

	var envs []string
	var args []string
	var err error
	if !strings.ContainsAny(rawCommand, `'"`) {
		envs, args = parseShellArgsWithoutQuotes(rawCommand)
	} else {
		// don't try to parse super long command lines, shellwords is pretty slow
		if len(rawCommand) > 100000 {
			return parsed
		}

		envs, args, err = shellwords.ParseWithEnvs(rawCommand)
		if err != nil {
			logger.Debugf("failed to parse shell words: %w: %s", err)
			return parsed
		}
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

	// use internal negate implementation
	if strings.HasSuffix(parsed.Command, "/negate") {
		ParseNegate(parsed)
	}

	return parsed
}

func parseShellArgsWithoutQuotes(rawCommand string) (envs []string, args []string) {
	splitted := strings.Fields(rawCommand)
	inEnv := true
	for _, s := range splitted {
		if inEnv {
			if strings.Contains(s, "=") {
				envs = append(envs, s)
				continue
			} else {
				inEnv = false
			}
		}
		args = append(args, s)
	}
	return
}
