package modgearman

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"
)

type answer struct {
	hostName           string
	serviceDescription string
	coreStartTime      float64
	startTime          float64
	finishTime         float64
	returnCode         int
	exitedOk           bool
	source             string
	output             string
	resultQueue        string
	active             string
	latency            float64
}

func (a *answer) String() string {
	result := fmt.Sprintf(
		"type=%s\n"+
			"host_name=%s\n"+
			"start_time=%f\n"+
			"finish_time=%f\n"+
			"return_code=%d\n"+
			"exited_ok=%d\n"+
			"source=%s\n"+
			"output=%s\n",
		a.active,
		a.hostName,
		a.startTime,
		a.finishTime,
		a.returnCode,
		bool2int(a.exitedOk),
		a.source,
		a.output,
	)
	if a.coreStartTime > 0 {
		result += fmt.Sprintf("core_start_time=%f\n", a.coreStartTime)
	}
	if a.serviceDescription != "" {
		result += fmt.Sprintf("service_description=%s\n", a.serviceDescription)
	}
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
	//first set the start time
	result.startTime = float64(time.Now().UnixNano()) / 1e9
	result.source = "Mod-Gearman Worker @ " + config.identifier
	result.active = "active"

	logger.Debug("new Job Received\n")
	logger.Debug(received)

	//hostname and core start time are the same in the result as in receive
	result.hostName = received.hostName
	result.coreStartTime = received.coreTime

	// check if the received assignment is too old
	//if maxAge set to 0 it does not get checked
	if config.maxAge > 0 {
		if result.startTime-result.coreStartTime > float64(config.maxAge) {
			logger.Debug("worker: readAndExecute: maxAge: job too old")
			result.output = "Could not Start Check In Time"
			return &result
		}
	}

	if received.timeout <= 0 {
		received.timeout = config.jobTimeout
	}
	if received.timeout <= 0 {
		received.timeout = 60
	}

	//run the command
	executeCommand(&result, received, config)

	// if this is a host call, no service_description is needed, else set the description
	// so the server recognizes the answer
	if received.serviceDescription != "" {
		result.serviceDescription = received.serviceDescription
	}

	//last set the finish time
	result.finishTime = float64(time.Now().UnixNano()) / 1e9
	result.resultQueue = received.resultQueue

	return &result
}

func checkRestrictPath(cmdString string, restrictPath []string) bool {
	if len(restrictPath) == 0 {
		return true
	}

	//check for restricted path
	splittedString := strings.Fields(cmdString)
	for _, v := range restrictPath {
		if strings.HasPrefix(splittedString[0], v) {
			return true
		}
	}

	return false
}

func executeInShell(cmdString string) bool {
	//if the command does not start with a / or a ., or has some of this chars inside it gets executed in the /bin/sh else as simple command
	if !strings.HasPrefix(cmdString, "/") && !strings.HasPrefix(cmdString, "./") {
		return true
	}
	if strings.ContainsAny(cmdString, "!$^&*()~[]\\|{\"};<>?`\\'") {
		return true
	}
	return false
}

//executes a command in the bash, returns whatever gets printed on the bash
//and as second value a status Code between 0 and 3
func executeCommand(result *answer, received *receivedStruct, config *configurationStruct) {

	result.returnCode = 3
	result.exitedOk = false
	if !checkRestrictPath(received.commandLine, config.restrictPath) {
		result.output = "command contains bad path"
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(received.timeout)*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if executeInShell(received.commandLine) {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", received.commandLine)
	} else {
		splitted := strings.Fields(received.commandLine)
		cmd = exec.CommandContext(ctx, splitted[0], splitted[1:]...)
	}

	//byte buffer for output
	var errbuf bytes.Buffer
	var outbuf bytes.Buffer
	cmd.Stdout = &outbuf
	cmd.Stderr = &errbuf
	cmd.Env = append(os.Environ())

	// prevent child from receiving signals meant for the worker only
	setSysProcAttr(cmd)

	err := cmd.Run()
	if err != nil && cmd.ProcessState == nil {
		logger.Errorf("Error in cmd.Run(): %s", err.Error())
		result.output = "UNKNOWN - cmd.Run(): " + err.Error()
		return
	}
	state := cmd.ProcessState
	if config.prometheusServer != "" {
		prometheusUserAndSystemTime(received.commandLine, state)
	}

	if ctx.Err() == context.DeadlineExceeded {
		result.returnCode = config.timeoutReturn
		if received.typ == "service" {
			logger.Infof("service check: %s - %s run into timeout after %d seconds", received.hostName, received.serviceDescription, received.timeout)
			result.output = fmt.Sprintf("(Service Check Timed Out On Worker: %s)", config.identifier)
		} else if received.typ == "host" {
			logger.Infof("host check: %s run into timeout after %d seconds", received.hostName, received.timeout)
			result.output = fmt.Sprintf("(Host Check Timed Out On Worker: %s)", config.identifier)
		} else {
			logger.Infof("%s with command %s run into timeout after %d seconds", received.typ, received.commandLine, received.timeout)
			result.output = fmt.Sprintf("(Check Timed Out On Worker: %s)", config.identifier)
		}
		return
	}

	if waitStatus, ok := state.Sys().(syscall.WaitStatus); ok {
		result.returnCode = waitStatus.ExitStatus()
		result.exitedOk = true
	}

	// extract stdout and stderr
	result.output = string(bytes.TrimSpace((bytes.Trim(outbuf.Bytes(), "\x00"))))
	if config.showErrorOutput && result.returnCode != 0 {
		error := string(bytes.TrimSpace((bytes.Trim(errbuf.Bytes(), "\x00"))))
		if error != "" {
			result.output += "\n[" + error + "]"
		}
	}
	result.output = strings.Replace(strings.Trim(result.output, "\r\n"), "\n", `\n`, len(result.output))

	if result.returnCode > 3 || result.returnCode < 0 {
		fixReturnCodes(result, config, state)
	}
}

func fixReturnCodes(result *answer, config *configurationStruct, state *os.ProcessState) {
	if result.returnCode == 126 {
		result.output = fmt.Sprintf("CRITICAL: Return code of 126 is out of bounds. Make sure the plugin you're trying to run is executable. (worker: %s)", config.identifier) + "\n" + result.output
		result.returnCode = 2
		return
	}
	if result.returnCode == 127 {
		result.output = fmt.Sprintf("CRITICAL: Return code of 127 is out of bounds. Make sure the plugin you're trying to run actually exists. (worker: %s)", config.identifier) + "\n" + result.output
		result.returnCode = 2
		return
	}
	if waitStatus, ok := state.Sys().(syscall.WaitStatus); ok {
		if waitStatus.Signaled() {
			result.output = fmt.Sprintf("CRITICAL: Return code of %d is out of bounds. Plugin exited by signal: %s. (worker: %s)", waitStatus.Signal(), waitStatus.Signal(), config.identifier) + "\n" + result.output
			result.returnCode = 2
			return
		}
	}
	result.output = fmt.Sprintf("CRITICAL: Return code of %d is out of bounds. (worker: %s)", result.returnCode, config.identifier) + "\n" + result.output
	result.returnCode = 3
}

func prometheusUserAndSystemTime(command string, state *os.ProcessState) {
	basename := getCommandBasename(command)
	userTimes.WithLabelValues(basename).Observe(state.UserTime().Seconds())
	systemTimes.WithLabelValues(basename).Observe(state.SystemTime().Seconds())

}

var reCmdEnvVar = regexp.MustCompile(`^[A-Za-z0-9_]+=("[^"]*"|'[^']*'|[^\s]*)\s+`)

func getCommandBasename(input string) string {
	l := len(input)
	for {
		input = reCmdEnvVar.ReplaceAllString(input, "")
		if len(input) == l {
			break
		}
		l = len(input)
	}
	args := strings.SplitN(input, " ", 2)
	paths := strings.Split(args[0], "/")
	return paths[len(paths)-1]
}
