package modgearman

import (
	"bytes"
	"context"
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

/**
* @ciphertext: base64 encoded, aes encrypted assignment
* @key: the aes key for decryption
* @return: answer, struct containing al information to be sent back to the server
*
 */
func readAndExecute(received *receivedStruct, config *configurationStruct) *answer {
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
			logger.Warnf("worker: maxAge: job too old: startTime: %s (threshold: %ds)", time.Unix(int64(result.coreStartTime), 0), config.maxAge)
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
func executeCommandLine(result *answer, received *receivedStruct, config *configurationStruct) {
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
		logger.Tracef("using shell for: %s", command.Command)
		execCmd(command, received, result, config)
	case Exec:
		result.execType = "exec"
		taskCounter.WithLabelValues(received.typ, result.execType).Inc()
		logger.Tracef("using exec for: %s", command.Command)
		execCmd(command, received, result, config)
	case Internal:
		result.execType = "internal"
		taskCounter.WithLabelValues(received.typ, result.execType).Inc()
		execInternal(result, command, received)
	default:
		logger.Panicf("unknown exec path: %v", command.ExecType)
	}
}

func execCmd(command *command, received *receivedStruct, result *answer, config *configurationStruct) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(received.timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, command.Command, command.Args...)

	// byte buffer for output
	var errbuf bytes.Buffer
	var outbuf bytes.Buffer
	cmd.Stdout = &outbuf
	cmd.Stderr = &errbuf
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
			cmd.Process.Kill()
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
		switch ctx.Err() {
		case context.DeadlineExceeded:
			// timeout
			processTimeoutKill(proc)
		case context.Canceled:
			// normal exit
			proc.Kill()
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
	result.output = string(bytes.TrimSpace((bytes.Trim(outbuf.Bytes(), "\x00"))))
	if config.showErrorOutput && result.returnCode != 0 {
		err := string(bytes.TrimSpace((bytes.Trim(errbuf.Bytes(), "\x00"))))
		if err != "" {
			result.output += "\n[" + err + "]"
		}
	}

	fixReturnCodes(result, config, state)
	result.output = strings.Replace(strings.Trim(result.output, "\r\n"), "\n", `\n`, len(result.output))
}

func execEPN(result *answer, cmd *command, received *receivedStruct) {
	logger.Tracef("using embedded perl for: %s", cmd.Command)
	err := executeWithEmbeddedPerl(cmd, result, received)
	if err != nil {
		if isRunning() {
			logger.Warnf("embedded perl failed for: %s: %w", cmd.Command, err)
		} else {
			logger.Debugf("embedded perl failed during shutdown for: %s: %w", cmd.Command, err)
		}
	}
}

func fixReturnCodes(result *answer, config *configurationStruct, state *os.ProcessState) {
	if result.returnCode >= 0 && result.returnCode <= 3 {
		return
	}
	if result.returnCode == exitCodeNotExecutable {
		result.output = fmt.Sprintf("UNKNOWN: Return code of %d is out of bounds. Make sure the plugin you're trying to run is executable. (worker: %s)", result.returnCode, config.identifier) + "\n" + result.output
		result.returnCode = 3
		return
	}
	if result.returnCode == exitCodeFileNotFound {
		result.output = fmt.Sprintf("UNKNOWN: Return code of %d is out of bounds. Make sure the plugin you're trying to run actually exists. (worker: %s)", result.returnCode, config.identifier) + "\n" + result.output
		result.returnCode = 3
		return
	}
	if waitStatus, ok := state.Sys().(syscall.WaitStatus); ok {
		if waitStatus.Signaled() {
			result.output = fmt.Sprintf("UNKNOWN: Return code of %d is out of bounds. Plugin exited by signal: %s. (worker: %s)", waitStatus.Signal(), waitStatus.Signal(), config.identifier) + "\n" + result.output
			result.returnCode = 3
			return
		}
	}
	result.output = fmt.Sprintf("CRITICAL: Return code of %d is out of bounds. (worker: %s)", result.returnCode, config.identifier) + "\n" + result.output
	result.returnCode = 3
}

func setTimeoutResult(result *answer, config *configurationStruct, received *receivedStruct, negate *Negate) {
	result.timedOut = true
	result.returnCode = config.timeoutReturn
	originalOutput := result.output
	switch received.typ {
	case "service":
		logger.Infof("service check: %s - %s run into timeout after %d seconds", received.hostName, received.serviceDescription, received.timeout)
		result.output = fmt.Sprintf("(Service Check Timed Out On Worker: %s)", config.identifier)
	case "host":
		logger.Infof("host check: %s run into timeout after %d seconds", received.hostName, received.timeout)
		result.output = fmt.Sprintf("(Host Check Timed Out On Worker: %s)", config.identifier)
	default:
		logger.Infof("%s with command %s run into timeout after %d seconds", received.typ, received.commandLine, received.timeout)
		result.output = fmt.Sprintf("(Check Timed Out On Worker: %s)", config.identifier)
	}
	if originalOutput != "" {
		result.output = fmt.Sprintf("%s\n%s", result.output, originalOutput)
	}
	if negate != nil {
		negate.SetTimeoutReturnCode(result)
	}
}

func setProcessErrorResult(result *answer, config *configurationStruct, err error) {
	if os.IsNotExist(err) {
		result.output = fmt.Sprintf("UNKNOWN: Return code of 127 is out of bounds. Make sure the plugin you're trying to run actually exists. (worker: %s)", config.identifier)
		result.returnCode = 3
		return
	}
	if os.IsPermission(err) {
		result.output = fmt.Sprintf("UNKNOWN: Return code of 126 is out of bounds. Make sure the plugin you're trying to run is executable. (worker: %s)", config.identifier)
		result.returnCode = 3
		return
	}
	if e, ok := err.(*os.PathError); ok {
		// catch some known errors and stop to prevent false positives
		switch e.Err {
		case syscall.EMFILE:
			// out of open files
			fallthrough
		case syscall.ENOMEM:
			// out of memory
			logger.Fatalf("system error, bailing out to prevent false positives: %w", err)
		}
	}
	logger.Warnf("system error: %w", err)
	result.returnCode = 3
	result.output = fmt.Sprintf("UNKNOWN: %s (worker: %s)", err.Error(), config.identifier)
}

var reCmdEnvVar = regexp.MustCompile(`^[A-Za-z0-9_]+=("[^"]*"|'[^']*'|[^\s]*)\s+`)

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
		l := len(input)
		for {
			input = reCmdEnvVar.ReplaceAllString(input, "")
			if len(input) == l {
				break
			}
			l = len(input)
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
		logger.Panicf("unhandled type: %v", com.ExecType)
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
