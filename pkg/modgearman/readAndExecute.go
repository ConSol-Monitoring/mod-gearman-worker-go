package modgearman

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"syscall"
	"time"
)

const (
	exitCodeNotExecutable = 126
	exitCodeFileNotFound  = 127
)

type answer struct {
	hostName           string
	serviceDescription string
	coreStartTime      float64
	startTime          float64
	finishTime         float64
	returnCode         int
	source             string
	output             string
	resultQueue        string
	active             string
	execType           string
	runUserDuration    float64
	runSysDuration     float64
	compileDuration    float64
	timedOut           bool
}

func (a *answer) String() string {
	optional := ""
	if a.serviceDescription != "" {
		optional += fmt.Sprintf("\nservice_description=%s", a.serviceDescription)
	}
	if a.coreStartTime > 0 {
		optional += fmt.Sprintf("\ncore_start_time=%f", a.coreStartTime)
	}
	result := fmt.Sprintf(`type=%s
host_name=%s%s
start_time=%f
finish_time=%f
return_code=%d
exited_ok=1
source=%s
output=%s
`,
		a.active,
		a.hostName,
		optional,
		a.startTime,
		a.finishTime,
		a.returnCode,
		a.source,
		strings.ReplaceAll(a.output, "\n", "\\n"),
	)

	return result
}

func readAndExecute(received *request, config *config) *answer {
	var result answer
	// first set the start time
	result.startTime = float64(time.Now().UnixNano()) / float64(time.Second)
	result.source = "Mod-Gearman Worker @ " + config.identifier
	result.active = "active"
	result.resultQueue = received.resultQueue

	// hostname and core start time are the same in the result as in receive
	result.hostName = received.hostName
	result.coreStartTime = received.coreTime

	// check if the received assignment is too old
	// if maxAge set to 0 it does not get checked
	if config.maxAge > 0 {
		if result.startTime-result.coreStartTime > float64(config.maxAge) {
			log.Warnf("worker: maxAge: job too old: startTime: %s (threshold: %ds)",
				time.Unix(int64(result.coreStartTime), 0), config.maxAge)
			result.output = fmt.Sprintf("Could not start check in time (worker: %s)", config.identifier)
			result.execType = "too_late"
			taskCounter.WithLabelValues(received.typ, result.execType).Inc()

			return &result
		}
	}

	if received.timeout <= 0 {
		received.timeout = config.jobTimeout
	}
	if received.timeout <= 0 {
		received.timeout = 60
	}

	// run the command
	executeCommandLine(&result, received, config)

	// if this is a host call, no service_description is needed, else set the description
	// so the server recognizes the answer
	if received.serviceDescription != "" {
		result.serviceDescription = received.serviceDescription
	}

	// last set the finish time
	result.finishTime = float64(time.Now().UnixNano()) / float64(time.Second)

	return &result
}

func checkRestrictPath(cmdString string, restrictPath []string) bool {
	if len(restrictPath) == 0 {
		return true
	}

	// check for restricted path
	splittedString := strings.Fields(cmdString)
	for _, v := range restrictPath {
		if strings.HasPrefix(splittedString[0], v) {
			return true
		}
	}

	return false
}

// executes a command in the bash, returns whatever gets printed on the bash
// and as second value a status Code between 0 and 3
func executeCommandLine(result *answer, received *request, config *config) {
	result.returnCode = 3
	command := parseCommand(received.commandLine, config)
	defer updatePrometheusExecMetrics(config, result, received, command)

	if !checkRestrictPath(received.commandLine, config.restrictPath) {
		result.execType = "bad_path"
		taskCounter.WithLabelValues(received.typ, result.execType).Inc()
		result.output = "command contains bad path"

		return
	}

	if command.Negate != nil && command.Negate.Timeout > 0 {
		received.timeout = command.Negate.Timeout
	}

	defer func() {
		if result.timedOut {
			setTimeoutResult(result, config, received, command.Negate)

			return
		}
		if command.Negate != nil {
			command.Negate.Apply(result)
		}
	}()

	switch command.ExecType {
	case EPN:
		result.execType = "epn"
		taskCounter.WithLabelValues(received.typ, result.execType).Inc()
		execEPN(result, command, received)
	case Shell:
		result.execType = "shell"
		taskCounter.WithLabelValues(received.typ, result.execType).Inc()
		log.Tracef("using shell for: %s", command.Command)
		execCmd(command, received, result, config)
	case Exec:
		result.execType = "exec"
		taskCounter.WithLabelValues(received.typ, result.execType).Inc()
		log.Tracef("using exec for: %s", command.Command)
		execCmd(command, received, result, config)
	case Internal:
		result.execType = "internal"
		taskCounter.WithLabelValues(received.typ, result.execType).Inc()
		execInternal(result, command, received)
	default:
		log.Panicf("unknown exec path: %v", command.ExecType)
	}
}

func execCmd(command *command, received *request, result *answer, config *config) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(received.timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, command.Command, command.Args...)

	// byte buffer for output
	var errBuf bytes.Buffer
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	cmd.Env = os.Environ()
	for key, val := range command.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, val))
	}

	// prevent child from receiving signals meant for the worker only
	setSysProcAttr(cmd)

	err := cmd.Start()
	if err != nil && cmd.ProcessState == nil {
		setProcessErrorResult(result, config, err)

		return
	}

	received.Cancel = func() {
		received.Canceled = true
		if cmd != nil && cmd.Process != nil {
			logDebug(cmd.Process.Kill())
		}
	}

	// https://github.com/golang/go/issues/18874
	// timeout does not work for child processes and/or if file handles are still open
	go func(proc *os.Process) {
		defer logPanicExit()
		<-ctx.Done() // wait till command runs into timeout or is finished (canceled)
		if proc == nil {
			return
		}
		ctxErr := ctx.Err()
		switch {
		case errors.Is(ctxErr, context.DeadlineExceeded):
			// timeout
			processTimeoutKill(proc)
		case errors.Is(ctxErr, context.Canceled):
			// normal exit
			logDebug(proc.Kill())
		}
	}(cmd.Process)

	err = cmd.Wait()
	cancel()
	if err != nil && cmd.ProcessState == nil {
		setProcessErrorResult(result, config, err)

		return
	}

	received.Cancel = nil

	state := cmd.ProcessState

	if ctx.Err() == context.DeadlineExceeded {
		result.timedOut = true

		return
	}

	if waitStatus, ok := state.Sys().(syscall.WaitStatus); ok {
		result.returnCode = waitStatus.ExitStatus()
	}

	if state != nil {
		result.runUserDuration = state.UserTime().Seconds()
		result.runSysDuration = state.SystemTime().Seconds()
	}

	// extract stdout and stderr
	result.output = string(bytes.TrimSpace((bytes.Trim(outBuf.Bytes(), "\x00"))))
	if config.showErrorOutput && result.returnCode != 0 {
		err := string(bytes.TrimSpace((bytes.Trim(errBuf.Bytes(), "\x00"))))
		if err != "" {
			result.output += "\n[" + err + "]"
		}
	}

	fixReturnCodes(result, config, state)
	result.output = strings.Replace(strings.Trim(result.output, "\r\n"), "\n", `\n`, len(result.output))
}

func execEPN(result *answer, cmd *command, received *request) {
	log.Tracef("using embedded perl for: %s", cmd.Command)
	err := executeWithEmbeddedPerl(cmd, result, received)
	if err != nil {
		if isRunning() {
			log.Warnf("embedded perl failed for: %s: %w", cmd.Command, err)
		} else {
			log.Debugf("embedded perl failed during shutdown for: %s: %w", cmd.Command, err)
		}
	}
}

func fixReturnCodes(result *answer, config *config, state *os.ProcessState) {
	if result.returnCode >= 0 && result.returnCode <= 3 {
		if config.workerNameInResult {
			outputparts := strings.SplitN(result.output, "|", 2)
			if len(outputparts) > 1 {
				result.output = fmt.Sprintf("%s (worker: %s) |%s", outputparts[0], config.identifier, outputparts[1])
			} else {
				result.output = fmt.Sprintf("%s (worker: %s)", result.output, config.identifier)
			}
		}

		return
	}
	if result.returnCode == exitCodeNotExecutable {
		result.output = fmt.Sprintf("UNKNOWN: Return code of %d is out of bounds. Make sure the plugin you're trying to run is executable. (worker: %s)",
			result.returnCode, config.identifier) + "\n" + result.output
		result.returnCode = 3

		return
	}
	if result.returnCode == exitCodeFileNotFound {
		result.output = fmt.Sprintf("UNKNOWN: Return code of %d is out of bounds. Make sure the plugin you're trying to run actually exists. (worker: %s)",
			result.returnCode, config.identifier) + "\n" + result.output
		result.returnCode = 3

		return
	}
	if waitStatus, ok := state.Sys().(syscall.WaitStatus); ok {
		if waitStatus.Signaled() {
			result.output = fmt.Sprintf("UNKNOWN: Return code of %d is out of bounds. Plugin exited by signal: %s. (worker: %s)",
				waitStatus.Signal(), waitStatus.Signal(), config.identifier) + "\n" + result.output
			result.returnCode = 3

			return
		}
	}
	result.output = fmt.Sprintf("CRITICAL: Return code of %d is out of bounds. (worker: %s)", result.returnCode, config.identifier) + "\n" + result.output
	result.returnCode = 3
}

func setTimeoutResult(result *answer, config *config, received *request, negate *Negate) {
	result.timedOut = true
	result.returnCode = config.timeoutReturn
	originalOutput := result.output
	switch received.typ {
	case "service":
		log.Infof("service check: %s - %s run into timeout after %d seconds",
			received.hostName, received.serviceDescription, received.timeout)
		result.output = fmt.Sprintf("(Service Check Timed Out On Worker: %s)", config.identifier)
	case "host":
		log.Infof("host check: %s run into timeout after %d seconds", received.hostName, received.timeout)
		result.output = fmt.Sprintf("(Host Check Timed Out On Worker: %s)", config.identifier)
	default:
		log.Infof("%s with command %s run into timeout after %d seconds",
			received.typ, received.commandLine, received.timeout)
		result.output = fmt.Sprintf("(Check Timed Out On Worker: %s)", config.identifier)
	}
	if originalOutput != "" {
		result.output = fmt.Sprintf("%s\n%s", result.output, originalOutput)
	}
	if negate != nil {
		negate.SetTimeoutReturnCode(result)
	}
}

func setProcessErrorResult(result *answer, config *config, err error) {
	if os.IsNotExist(err) {
		result.output = fmt.Sprintf("UNKNOWN: Return code of 127 is out of bounds. Make sure the plugin you're trying to run actually exists. (worker: %s)",
			config.identifier)
		result.returnCode = 3

		return
	}
	if os.IsPermission(err) {
		result.output = fmt.Sprintf("UNKNOWN: Return code of 126 is out of bounds. Make sure the plugin you're trying to run is executable. (worker: %s)",
			config.identifier)
		result.returnCode = 3

		return
	}

	// catch some known errors and stop to prevent false positives
	switch {
	case errors.Is(err, syscall.EMFILE):
		// out of open files
		log.Fatalf("system error out of files, bailing out to prevent false positives: %w: %s", err, err.Error())
	case errors.Is(err, syscall.ENOMEM):
		// out of memory
		log.Fatalf("system error out of memory, bailing out to prevent false positives: %w: %s", err, err.Error())
	}

	log.Warnf("system error: %w: %s", err, err.Error())
	result.returnCode = 3
	result.output = fmt.Sprintf("UNKNOWN: %s (worker: %s)", err.Error(), config.identifier)
}

var reCmdEnvVar = regexp.MustCompile(`^[A-Za-z0-9_]+=("[^"]*"|'[^']*'|\S*)\s+`)

// returns basename and full qualifier for command line
func getCommandQualifier(com *command) string {
	qualifier := ""
	var args []string
	switch com.ExecType {
	case Shell:
		if len(com.Args) < 2 {
			return com.Command
		}
		input := com.Args[1]
		length := len(input)
		for {
			input = reCmdEnvVar.ReplaceAllString(input, "")
			if len(input) == length {
				break
			}
			length = len(input)
		}
		args = strings.SplitN(input, " ", 3)
		qualifier = path.Base(args[0])
	case Exec, EPN:
		qualifier = path.Base(com.Command)
		args = com.Args
	case Internal:
		qualifier = fmt.Sprintf("%T", com.InternalCheck)
		qualifier = strings.TrimPrefix(qualifier, "*modgearman.")
		args = com.Args
	default:
		log.Panicf("unhandled type: %v", com.ExecType)
	}

	switch qualifier {
	case "python", "python2", "python3", "bash", "sh", "perl":
		// add basename of first argument
		for _, arg1 := range args {
			switch arg1 {
			case "-c", "-w":
				// skip some known none-filenames
				continue
			}
			arg1paths := strings.Split(arg1, "/")
			arg1base := arg1paths[len(arg1paths)-1]
			qualifier = fmt.Sprintf("%s %s", qualifier, arg1base)

			break
		}
	default:
	}

	return qualifier
}
