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

func mainSignalHandler(sig os.Signal, config *config) MainStateType {
	switch sig {
	case syscall.SIGTERM:
		log.Infof("got sigterm, quiting gracefully")

		return ShutdownGraceFully
	case syscall.SIGINT, os.Interrupt:
		log.Infof("got sigint, quitting")

		return Shutdown
	case syscall.SIGHUP:
		log.Infof("got sighup, reloading configuration...")

		return Reload
	case syscall.SIGUSR1:
		log.Errorf("requested thread dump via signal %s", sig)
		logThreadDump()

		return Resume
	case syscall.SIGUSR2:
		if config.flagMemProfile == "" {
			log.Errorf("requested memory profile, but flag -memprofile missing")

			return (Resume)
		}
		file, err := os.Create(config.flagMemProfile)
		if err != nil {
			log.Errorf("could not create memory profile: %w", err)
		}
		defer file.Close()
		runtime.GC()
		if err := pprof.WriteHeapProfile(file); err != nil {
			log.Errorf("could not write memory profile: %w", err)
		}
		log.Warnf("memory profile written to: %s", config.flagMemProfile)

		return (Resume)
	default:
		log.Warnf("Signal not handled: %v", sig)
	}

	return Resume
}

func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}
}

func processTimeoutKill(proc *os.Process) {
	go func(pid int) {
		// kill the process itself and the hole process group
		logDebug(syscall.Kill(-pid, syscall.SIGTERM))
		time.Sleep(1 * time.Second)

		logDebug(syscall.Kill(-pid, syscall.SIGINT))
		time.Sleep(1 * time.Second)

		logDebug(syscall.Kill(-pid, syscall.SIGKILL))
	}(proc.Pid)
}

func getMaxOpenFiles() uint64 {
	var rLimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		log.Warnf("cannot fetch open files limit: %w", err)
	}

	return rLimit.Cur
}
