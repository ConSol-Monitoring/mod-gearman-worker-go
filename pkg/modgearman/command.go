package modgearman

import (
	"strings"

	"github.com/sni/shelltoken"
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

	// Internal is for internal checks
	Internal
)

type command struct {
	ExecType      CommandExecType
	Command       string
	Args          []string
	Env           map[string]string
	Negate        *Negate
	InternalCheck InternalCheck
}

func parseCommand(rawCommand string, config *config) *command {
	parsed := &command{
		ExecType: Shell,
		Command:  rawCommand,
		Args:     make([]string, 0),
		Env:      make(map[string]string),
	}

	envs, args, err := shelltoken.SplitLinux(rawCommand)
	if err != nil {
		log.Tracef("failed to parse shell args: %w: %s", err, err.Error())
		parsed.Args = []string{"-c", parsed.Command}
		parsed.Command = "/bin/sh"

		return parsed
	}

	parsed.Command = args[0]
	parsed.Args = args[1:]
	parsed.ExecType = Exec
	parsed.appendEnv(envs)

	if fileUsesEmbeddedPerl(parsed.Command, config) {
		parsed.ExecType = EPN
	}

	// use internal negate implementation
	if config.internalNegate && strings.HasSuffix(parsed.Command, "/negate") {
		ParseNegate(parsed)
	}

	// use internal check_dummy implementation
	if config.internalCheckDummy && strings.HasSuffix(parsed.Command, "/check_dummy") {
		parsed.InternalCheck = &InternalCheckDummy{}
		parsed.ExecType = Internal
	}

	// use internal check_nsc_web implementation
	if config.internalCheckNscWeb && strings.HasSuffix(parsed.Command, "/check_nsc_web") {
		parsed.InternalCheck = &InternalCheckNSCWeb{}
		parsed.ExecType = Internal
	}

	if config.internalCheckPrometheus && strings.HasSuffix(parsed.Command, "/check_prometheus") {
		parsed.InternalCheck = &internalCheckPrometheus{}
		parsed.ExecType = Internal
	}

	return parsed
}

func (com *command) appendEnv(envs []string) {
	for _, env := range envs {
		splitted := strings.SplitN(env, "=", 2)
		com.Env[splitted[0]] = splitted[1]
	}
}
