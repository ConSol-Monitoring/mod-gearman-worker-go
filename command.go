package modgearman

import (
	"regexp"
	"strings"
	"utils"
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

		args = utils.Tokenize(rawCommand)
		envs, args = extractEnvFromArgv(args)
		args, err = utils.TrimQuotesAll(args)
		if err != nil {
			logger.Debugf("failed to parse shell args: %w: %s", err, err.Error())
			return parsed
		}
	}
	parsed.Command = args[0]
	parsed.Args = args[1:]
	parsed.ExecType = Exec
	for _, env := range envs {
		splitted := strings.SplitN(env, "=", 2)
		val, _ := utils.TrimQuotes(splitted[1])
		parsed.Env[splitted[0]] = val
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

func parseShellArgsWithoutQuotes(rawCommand string) (envs []string, args []string) {
	splitted := strings.Fields(rawCommand)

	return extractEnvFromArgv(splitted)
}

func extractEnvFromArgv(argv []string) (envs, args []string) {
	inEnv := true
	for _, s := range argv {
		if inEnv {
			if strings.Contains(s, "=") {
				envs = append(envs, s)
				continue
			}
			inEnv = false
		}
		args = append(args, s)
	}
	return
}
