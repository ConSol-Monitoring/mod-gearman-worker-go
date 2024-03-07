package modgearman

import (
	"os"
	"os/exec"
	"syscall"
)

func setupUsrSignalChannel(osSignalUsrChannel chan os.Signal) {
	// not supported on windows
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
	default:
		logger.Warnf("Signal not handled: %v", sig)
	}
	return Resume
}

func setSysProcAttr(cmd *exec.Cmd) {
	// not supported on windows
}

func processTimeoutKill(p *os.Process) {
	p.Kill()
}

func getMaxOpenFiles() uint64 {
	return 0
}
