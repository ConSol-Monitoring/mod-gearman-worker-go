package modgearman

import (
	"regexp"
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

var (
	shellCharacters = "!$^&*()~[]\\|{};<>?`"
	reCommandQuotes = regexp.MustCompile("'[^']*'|\"[^$`\"]*\"")
)

type command struct {
	ExecType      CommandExecType
	Command       string
	Args          []string
	Env           map[string]string
	Negate        *Negate
	InternalCheck InternalCheck
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

	// remove quoted strings from command line, then check if we find shell characters
	testCommand := reCommandQuotes.ReplaceAllString(rawCommand, "")
	if strings.ContainsAny(testCommand, shellCharacters) {
		return parsed
	}

	envs, args, err := shelltoken.Parse(rawCommand)
	if err != nil {
		logger.Debugf("failed to parse shell args: %w: %s", err, err.Error())
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

	return parsed
}
