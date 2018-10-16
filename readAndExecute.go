package main

import (
	"bytes"
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
}

func (a *answer) String() string {
	//service description is not available for hosts -> must not appear in answer
	serviceDescription := "service_description=" + a.serviceDescription + "\n"
	if a.serviceDescription == "" {
		serviceDescription = ""
	}
	//exited_ok is bool but we need the int representation here
	var exited int
	if a.exitedOk {
		exited = 1
	}
	return fmt.Sprintf(
		"host_name=%s\n"+
			serviceDescription+
			"core_start_time=%f\n"+
			"start_time=%f\n"+
			"finish_time=%f\n"+
			"return_code=%d\n"+
			"exited_ok=%d\n"+
			"source=%s\n"+
			"output=%s\n"+
			"type=%s\n",
		a.hostName,
		a.coreStartTime,
		a.startTime,
		a.finishTime,
		a.returnCode,
		exited,
		a.source,
		a.output,
		a.active)
}

/**
* @ciphertext: base64 encoded, aes encrypted assignment
* @key: the aes key for decryption
* @return: answer, struct containing al information to be sent back to the server
*
 */
func readAndExecute(received *receivedStruct, key []byte, config *configurationStruct) *answer {
	var result answer
	//first set the start time
	result.startTime = float64(time.Now().UnixNano()) / 1e9
	result.source = "Mod-Gearman Worker @ " + config.identifier

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

	//get the timeout time, and execute the command
	timeout := received.timeout
	if timeout == 0 {
		timeout = config.jobTimeout
	}
	commandOutput, statusCode := executeCommandWithTimeout(received.commandLine, timeout, config)

	// if this is a host call, no service_description is needed, else set the description
	// so the server recognizes the answer
	if received.serviceDescription != "" {
		result.serviceDescription = received.serviceDescription
	}

	// if the statuscode is 4 we ran into a timeout,
	// status 4 is invalid and needs to be 3, but after timeout we set
	// the exited to false
	if statusCode == 4 {
		result.exitedOk = false
		result.returnCode = config.timeoutReturn
	} else if statusCode == 25 && config.workaroundRc25 {
		return &answer{}
	} else {
		result.returnCode = statusCode
		result.exitedOk = true
	}

	//set the output of the command
	result.output = commandOutput

	//last set the finish time
	result.finishTime = float64(time.Now().UnixNano()) / 1e9
	result.resultQueue = received.resultQueue

	return &result
}

func containsBadPathOrChars(cmdString string, restrictPath []string, restrictChars []string) bool {
	//check for restricted path
	splittedString := strings.Split(cmdString, " ")
	for _, v := range restrictPath {
		if !strings.HasPrefix(splittedString[0], v) {
			return true
		}
	}

	//check for forbidden characters, only if
	if len(restrictPath) != 0 {
		for _, v := range restrictChars {
			if strings.Contains(cmdString, v) {
				return true
			}
		}
	}
	return false
}

func executeInShell(command string, cmdString string) bool {
	returnvalue := true
	//if the command does not start with a / or a ., or has some of this chars inside it gets executed in the /bin/sh else as simple command
	if strings.HasPrefix(command, "/") || strings.HasPrefix(command, ".") {
		returnvalue = false
	}
	if strings.ContainsAny(cmdString, "!$^&*()~[]\\|{\"};<>?`\\'") {
		returnvalue = true
	}
	return returnvalue
}

//executes a command in the bash, returns whatever gets printed on the bash
//and as second value a status Code between 0 and 3
//after seconds in timeOut kills the process and returns status code 4
func executeCommandWithTimeout(cmdString string, timeOut int, config *configurationStruct) (string, int) {
	var result string

	if containsBadPathOrChars(cmdString, config.restrictPath, config.restrictCommandCharacters) {
		return "command contains bad path or characters", 2
	}

	command, args := splitCommandArguments(cmdString)
	runWithShell := executeInShell(command, cmdString)

	var cmd *exec.Cmd
	if runWithShell {
		cmd = exec.Command("/bin/sh", "-c", cmdString)
	} else {
		cmd = exec.Command(command, args...)
	}

	//byte buffer for output
	var errbuff bytes.Buffer
	var outbuf bytes.Buffer
	cmd.Stdout = &outbuf
	cmd.Stderr = &errbuff
	cmd.Env = append(os.Environ())

	// prevent child from receiving signals meant for the worker only
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}

	if err := cmd.Start(); err != nil {
		logger.Error("Error starting command: ", err)
		return "ERROR CMD Start: " + err.Error(), 3
	}

	done := make(chan error)
	//go routine listening for the exit of the command, then writes the status to chanel done
	go func() {
		defer logPanicExit()
		done <- cmd.Wait()
	}()

	timeoutTimer := time.After(time.Duration(timeOut) * time.Second)

	select {
	case <-timeoutTimer:
		//we timed out!
		logger.Infof("command: %s run into timeout after %d seconds", cmdString, timeOut)
		cmd.Process.Signal(syscall.SIGKILL)
		state, _ := cmd.Process.Wait()
		prometheusUserAndSystemTime(cmd, command, state)
		cmd.Process.Kill()
		return "timeout", 4 //return code 4 as identifier that we ran in an timeout
	case err := <-done:
		prometheusUserAndSystemTime(cmd, command, nil)
		//command completed in time
		result = outbuf.String()
		if errbuff.String() != "" && config.showErrorOutput {
			result = "[ " + errbuff.String() + " ]"
		}
		statusCode := 0
		if err != nil {
			//get the status code via exec:
			if exiterr, ok := err.(*exec.ExitError); ok {
				if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
					statusCode = status.ExitStatus()
				}
			} else {
				logger.Error("cmd.Wait: ", err)
				fmt.Println("exitTime: ", exiterr.UserTime())
			}

			result = err.Error() + " " + result
			if statusCode > 4 || statusCode < 0 {
				statusCode = 3
			}
		}
		result = strings.Replace(result, "\n", "", len(result))
		return result, statusCode
	}
}

func prometheusUserAndSystemTime(cmd *exec.Cmd, command string, state *os.ProcessState) {
	if state == nil && cmd.ProcessState != nil {
		state = cmd.ProcessState
	}
	if state == nil {
		return
	}

	basename := getCommandBasename(command)
	userTimes.WithLabelValues(basename).Observe(state.UserTime().Seconds())
	systemTimes.WithLabelValues(basename).Observe(state.SystemTime().Seconds())

}

func splitCommandArguments(input string) (string, []string) {
	splitted := strings.Split(input, " ")
	command := splitted[0]
	args := splitted[1:]
	return command, args
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
