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

	fileUsesEPNCache = make(map[string]bool)
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
		err = fmt.Errorf("failed to create epn socket: %w", err)
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		logger.Errorf("epn startup error: %s", err)
		cleanExit(ExitCodeError)
	}
	args = append(args, socketPath.Name())
	os.Remove(socketPath.Name())

	cmd := exec.Command(config.p1File, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		err = fmt.Errorf("failed to connect to stdout: %w", err)
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		logger.Errorf("epn startup error: %s", err)
		cleanExit(ExitCodeError)
	}
	stdoutScanner := bufio.NewScanner(stdout)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		err = fmt.Errorf("failed to connect to stderr: %w", err)
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		logger.Errorf("epn startup error: %s", err)
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
		err = fmt.Errorf("failed to start epn worker: %w", err)
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		logger.Errorf("epn startup error: %s", err)
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
			err = fmt.Errorf("timeout (%s) while waiting for epn socket", ePNStartTimeout)
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
			logger.Errorf("epn startup error: %s", err)
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
	cached, ok := fileUsesEPNCache[file]
	if ok {
		return cached
	}
	fileUsesEPN := detectFileUsesEmbeddedPerl(file, config)
	fileUsesEPNCache[file] = fileUsesEPN
	return fileUsesEPN
}

func detectFileUsesEmbeddedPerl(file string, config *configurationStruct) bool {
	readFile, err := os.Open(file)
	if err != nil {
		logger.Warnf("failed to open %s: %s", file, err.Error())
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

func executeWithEmbeddedPerl(bin string, args []string, result *answer, received *receivedStruct) error {
	msg, err := json.Marshal(ePNMsg{
		Bin:     bin,
		Args:    args,
		Env:     map[string]string{},
		Timeout: received.timeout,
	})
	if err != nil {
		return fmt.Errorf("json error: %w", err)
	}

	c, err := net.Dial("unix", ePNServerSocket.Name())
	if err != nil {
		return fmt.Errorf("sending to epn server failed: %w", err)
	}
	defer c.Close()

	msg = append(msg, '\n')
	_, err = c.Write(msg)
	if err != nil {
		return fmt.Errorf("sending to epn server failed: %w", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, c)

	res := ePNRes{}
	err = json.Unmarshal(buf.Bytes(), &res)
	if err != nil {
		return fmt.Errorf("json unpacking failed: %w", err)
	}

	result.output = res.Stdout
	result.returnCode = res.RC
	result.compileDuration = res.CompileDuration
	result.runUserDuration = res.CPUUser

	return nil
}
