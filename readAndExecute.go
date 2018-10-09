package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

type answer struct {
	host_name           string
	service_description string
	core_start_time     float64
	start_time          float64
	finish_time         float64
	return_code         int
	exited_ok           bool
	source              string
	output              string
	result_queue        string
	active              string
}

func (a *answer) String() string {
	//service description is not available for hosts -> must not appear in answer
	service_description := "service_description=" + a.service_description + "\n"
	if a.service_description == "" {
		service_description = ""
	}
	//exited_ok is bool but we need the int representation here
	var exited int
	if a.exited_ok {
		exited = 1
	}
	return fmt.Sprintf(
		"host_name=%s\n"+
			service_description+
			"core_start_time=%f\n"+
			"start_time=%f\n"+
			"finish_time=%f\n"+
			"return_code=%d\n"+
			"exited_ok=%d\n"+
			"source=%s\n"+
			"output=%s\n"+
			"type=%s\n",
		a.host_name,
		a.core_start_time,
		a.start_time,
		a.finish_time,
		a.return_code,
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
func readAndExecute(received *receivedStruct, key []byte) *answer {
	var result answer
	//first set the start time
	result.start_time = float64(time.Now().UnixNano()) / 1e9
	result.source = "Mod-Gearman Worker @ " + config.identifier

	logger.Debug("new Job Received\n")
	logger.Debug(received)

	//hostname and core start time are the same in the result as in receive
	result.host_name = received.host_name
	result.core_start_time = received.core_time

	// check if the received assignment is too old
	//if maxAge set to 0 it does not get checked
	if config.max_age != 0 {
		if result.start_time-result.core_start_time > float64(config.max_age) {
			logger.Debug("worker: readAndExecute: maxAge: job too old")
			result.output = "Could not Start Check In Time"
			return &result
		}
	}

	//get the timeout time, and execute the command
	timeout := received.timeout
	if timeout == 0 {
		timeout = config.job_timeout
	}
	commandOutput, statusCode := executeCommandWithTimeout(received.command_line, timeout)

	// if this is a host call, no service_description is needed, else set the description
	// so the server recognizes the answer
	if received.service_description != "" {
		result.service_description = received.service_description
	}

	// if the statuscode is 4 we ran into a timeout,
	// status 4 is invalid and needs to be 3, but after timeout we set
	// the exited to false
	if statusCode == 4 {
		result.exited_ok = false
		result.return_code = config.timeout_return
	} else if statusCode == 25 && config.workaround_rc_25 {
		return &answer{}
	} else {
		result.return_code = statusCode
		result.exited_ok = true
	}

	//set the output of the command
	result.output = commandOutput

	//last set the finish time
	result.finish_time = float64(time.Now().UnixNano()) / 1e9
	result.result_queue = received.result_queue

	return &result
}

//executes a command in the bash, returns whatever gets printed on the bash
//and as second value a status Code between 0 and 3
//after seconds in timeOut kills the process and returns status code 4
func executeCommandWithTimeout(cmdString string, timeOut int) (string, int) {
	var result string

	//check for restricted path
	splittedString := strings.Split(cmdString, " ")
	for _, v := range config.restrict_path {
		if !strings.HasPrefix(splittedString[0], v) {
			return "try to access forbidden path", 2
		}
	}

	//check for forbidden characters, only if
	if len(config.restrict_path) != 0 {
		for _, v := range config.restrict_command_characters {
			if strings.Contains(cmdString, v) {
				return ("character " + v + " not allowed!!"), 2
			}
		}
	}

	command, args := splitCommandArguments(cmdString)
	cmd := exec.Command("/bin/sh", "-c", cmdString)

	//if the command does not start with a / or a ., or has some of this chars inside it gets executed in the /bin/sh else as simple command
	if strings.HasPrefix(command, "/") || strings.HasPrefix(command, ".") {
		cmd = exec.Command(command, args...)
		// logger.Info(args)
	}
	for _, v := range []string{"!", "$", "^", "&", "*", "(", ")", "~", "[", "]", "\\", "|", "{", "\"", "}", ";", "<", ">", "?", "`", "\\", "'"} {
		if strings.Contains(cmdString, v) {
			cmd = exec.Command("/bin/sh", "-c", cmdString)
		}
	}

	//byte buffer for output
	var errbuff bytes.Buffer
	var outbuf bytes.Buffer
	cmd.Stdout = &outbuf
	cmd.Stderr = &errbuff
	cmd.Env = append(os.Environ())

	if err := cmd.Start(); err != nil {
		logger.Error("Error starting command: ", err)
		return "ERROR CMD Start: " + err.Error(), 3
	}

	done := make(chan error)
	//go routine listening for the exit of the command, then writes the status to chanel done
	go func() { done <- cmd.Wait() }()

	timeoutTimer := time.After(time.Duration(timeOut) * time.Second)

	select {
	case <-timeoutTimer:
		//we timed out!
		logger.Debug("Timeout!!!")
		cmd.Process.Kill()
		return "timeout", 4 //return code 4 as identifier that we ran in an timeout
	case err := <-done:
		userTime := (float64(cmd.ProcessState.UserTime()/time.Nanosecond) / 1e9)
		systemTime := (float64(cmd.ProcessState.SystemTime()/time.Nanosecond) / 1e9)

		logger.Infof("Command: %s, userTime: %f, UserTime(): %s", command, userTime, (cmd.ProcessState.UserTime().String()))

		userTimes.WithLabelValues(getCommand(command)).Observe(userTime)
		systemTimes.WithLabelValues(getCommand(command)).Observe(systemTime)

		//command completed in time
		result = outbuf.String()
		if errbuff.String() != "" && config.show_error_output {
			result += "[ " + errbuff.String() + " ]"
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

func splitCommandArguments(input string) (string, []string) {
	splitted := strings.Split(input, " ")
	command := splitted[0]
	args := splitted[1:]
	return command, args
}

func getCommand(input string) string {
	splitted := strings.Split(input, "/")
	if len(splitted) <= 1 {
		return input
	}
	return splitted[len(splitted)-1]

}
