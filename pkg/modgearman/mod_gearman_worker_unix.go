//go:build !windows

package modgearman

import (
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"syscall"
	"time"
)

func setupUsrSignalChannel(osSignalUsrChannel chan os.Signal) {
	signal.Notify(osSignalUsrChannel, syscall.SIGUSR1)
	signal.Notify(osSignalUsrChannel, syscall.SIGUSR2)
}

func mainSignalHandler(sig os.Signal, config *configurationStruct) MainStateType {
	switch sig {
	case syscall.SIGTERM:
		logger.Infof("got sigterm, quiting gracefully")
		return ShutdownGraceFully
	case syscall.SIGINT:
		fallthrough
	case os.Interrupt:
		logger.Infof("got sigint, quitting")
		return Shutdown
	case syscall.SIGHUP:
		logger.Infof("got sighup, reloading configuration...")
		return Reload
	case syscall.SIGUSR1:
		logger.Errorf("requested thread dump via signal %s", sig)
		logThreaddump()
		return Resume
	case syscall.SIGUSR2:
		if config.flagMemProfile == "" {
			logger.Errorf("requested memory profile, but flag -memprofile missing")
			return (Resume)
		}
		f, err := os.Create(config.flagMemProfile)
		if err != nil {
			logger.Errorf("could not create memory profile: %w", err)
		}
		defer f.Close()
		runtime.GC()
		if err := pprof.WriteHeapProfile(f); err != nil {
			logger.Errorf("could not write memory profile: %w", err)
		}
		logger.Warnf("memory profile written to: %s", config.flagMemProfile)
		return (Resume)
	default:
		logger.Warnf("Signal not handled: %v", sig)
	}
	return Resume
}

func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}
}

func processTimeoutKill(p *os.Process) {
	go func(pid int) {
		// kill the process itself and the hole process group
		syscall.Kill(-pid, syscall.SIGTERM)
		time.Sleep(1 * time.Second)

		syscall.Kill(-pid, syscall.SIGINT)
		time.Sleep(1 * time.Second)

		syscall.Kill(-pid, syscall.SIGKILL)
	}(p.Pid)
}

func getMaxOpenFiles() uint64 {
	var rLimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		logger.Warnf("cannot fetch open files limit: %w", err)
	}
	return rLimit.Cur
}
