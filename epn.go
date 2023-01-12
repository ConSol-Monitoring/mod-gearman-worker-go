package modgearman

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	// ePNStartTimeout is the amount of time we wait for the socket to appear
	ePNStartTimeout = 5 * time.Second

	// ePNStartRetryInterval is the interval at which the socket is checked
	ePNStartRetryInterval = 50 * time.Millisecond
)

var (
	ePNServerProcess *exec.Cmd
	ePNServerSocket  *os.File

	// ePNFilePrefix contains prefixes to look for explicit epn flag
	ePNFilePrefix = []string{"# nagios:", "# naemon:", "# icinga:"}
)

func startEmbeddedPerl(config *configurationStruct) {
	ePNServerProcess = nil
	ePNServerSocket = nil
	if !config.enableEmbeddedPerl {
		return
	}
	logger.Debugf("starting embedded perl worker")
	args := make([]string, 0)
	if config.usePerlCache {
		args = append(args, "-c")
	}
	if config.debug >= LogLevelDebug {
		args = append(args, "-v")
	}
	if config.debug >= LogLevelTrace {
		args = append(args, "-vv")
	}
	socketPath, err := os.CreateTemp("", "mod_gearman_worker_epn*.socket")
	ePNServerSocket = socketPath
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create epn socket: %s\n", err.Error())
		cleanExit(ExitCodeError)
	}
	args = append(args, socketPath.Name())
	os.Remove(socketPath.Name())

	cmd := exec.Command(config.p1File, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to connect to stdout: %s\n", err.Error())
		cleanExit(ExitCodeError)
	}
	stdoutScanner := bufio.NewScanner(stdout)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to connect to stderr: %s\n", err.Error())
		cleanExit(ExitCodeError)
	}
	stderrScanner := bufio.NewScanner(stderr)
	go func() {
		for stdoutScanner.Scan() {
			logger.Debugf("%s", stdoutScanner.Text())
		}
	}()
	go func() {
		for stderrScanner.Scan() {
			logger.Errorf("%s", stderrScanner.Text())
		}
	}()

	err = cmd.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to start epn worker: %s\n", err.Error())
		cleanExit(ExitCodeError)
	}
	ePNServerProcess = cmd

	// wait till socket appears
	ticker := time.NewTicker(ePNStartRetryInterval)
	timeout := time.NewTimer(ePNStartTimeout)
	keepTrying := true
	for keepTrying {
		select {
		case <-timeout.C:
			fmt.Fprintf(os.Stderr, "Error: failed to open epn socket\n")
			cleanExit(ExitCodeError)
		case <-ticker.C:
			_, err := os.Stat(socketPath.Name())
			if err == nil {
				keepTrying = false
			}
		}
	}
	ticker.Stop()
	timeout.Stop()
}

func stopEmbeddedPerl() {
	if ePNServerProcess == nil {
		return
	}
	if ePNServerProcess.Process == nil {
		return
	}

	ePNServerProcess.Process.Signal(os.Interrupt)
	ePNServerProcess.Process.Release()
	ePNServerProcess = nil
	if ePNServerSocket != nil {
		os.Remove(ePNServerSocket.Name())
	}
	logger.Debugf("epn worker shutdown complete")
}

func fileUsesEmbeddedPerl(file string, config *configurationStruct) bool {
	if !config.enableEmbeddedPerl {
		return false
	}
	readFile, err := os.Open(file)
	if err != nil {
		return false
	}
	defer readFile.Close()
	fileScanner := bufio.NewScanner(readFile)
	fileScanner.Split(bufio.ScanLines)
	linesRead := 0
	for linesRead < 10 && fileScanner.Scan() {
		line := fileScanner.Text()
		linesRead++
		if linesRead == 1 {
			// check if first line contains perl shebang
			if !strings.Contains(line, "/bin/perl") {
				return false
			}
			continue
		}
		if !strings.HasPrefix(line, "# ") {
			continue
		}
		for _, prefix := range ePNFilePrefix {
			if strings.HasPrefix(line, prefix) {
				line = strings.TrimPrefix(line, prefix)
				line = strings.TrimSpace(line)
				switch line[0:1] {
				case "+":
					return true
				case "-":
					return false
				}
			}
		}
	}

	// nothing explicitly found, fallback to config default
	return config.useEmbeddedPerlImplicitly
}

type ePNMsg struct {
	Bin     string            `json:"bin"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	Timeout int               `json:"timeout"`
}

type ePNRes struct {
	RC              int     `json:"rc"`
	Stdout          string  `json:"stdout"`
	RunDuration     float64 `json:"run_duration"`
	CompileDuration float64 `json:"compile_duration"`
	CPUUser         float64 `json:"cpu_user"`
}

func executeWithEmbeddedPerl(bin string, args []string, result *answer, received *receivedStruct, config *configurationStruct) bool {
	msg, err := json.Marshal(ePNMsg{
		Bin:     bin,
		Args:    args,
		Env:     map[string]string{},
		Timeout: received.timeout,
	})
	if err != nil {
		logger.Errorf("json error: %s", err)
		return false
	}

	c, err := net.Dial("unix", ePNServerSocket.Name())
	if err != nil {
		logger.Errorf("sending to epn server failed: %s", err)
		return false
	}
	defer c.Close()

	msg = append(msg, '\n')
	_, err = c.Write(msg)
	if err != nil {
		logger.Errorf("sending to epn server failed: %s", err)
		return false
	}

	var buf bytes.Buffer
	io.Copy(&buf, c)

	res := ePNRes{}
	err = json.Unmarshal(buf.Bytes(), &res)
	if err != nil {
		logger.Errorf("json unpacking failed: %s", err)
		return false
	}

	result.output = res.Stdout
	result.returnCode = res.RC

	if config.prometheusServer != "" {
		basename := getCommandBasename(bin)
		userTimes.WithLabelValues(basename).Observe(res.CPUUser)
		systemTimes.WithLabelValues(basename).Observe(0)
	}

	return true
}
