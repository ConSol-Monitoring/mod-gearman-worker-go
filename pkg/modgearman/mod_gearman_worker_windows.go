package modgearman

import (
	"os"
	"os/exec"
	"syscall"
)

func setupUsrSignalChannel(_ chan os.Signal) {
	// not supported on windows
}

func mainSignalHandler(sig os.Signal, _ *config) MainStateType {
	switch sig {
	case syscall.SIGTERM:
		log.Infof("got sigterm, exiting gracefully")

		return ShutdownGraceFully
	case syscall.SIGINT, os.Interrupt:
		log.Infof("got sigint, quitting")

		return Shutdown
	case syscall.SIGHUP:
		log.Infof("got sighup, reloading configuration...")

		return Reload
	case syscall.SIGSEGV:
		logThreadDump()
		os.Exit(1)
	default:
		log.Warnf("Signal not handled: %v", sig)
	}

	return Resume
}

func setSysProcAttr(_ *exec.Cmd) {
	// not supported on windows
}

func processTimeoutKill(p *os.Process) {
	logDebug(p.Kill())
}

func getMaxOpenFiles() uint64 {
	return 0
}
