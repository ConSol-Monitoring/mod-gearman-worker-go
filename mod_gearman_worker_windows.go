package modgearman

import (
	"net"
	"os"
	"os/exec"
	"syscall"
)

func setupUsr1Channel(osSignalUsrChannel chan os.Signal) {
	// not supported on windows
}

func mainSignalHandler(sig os.Signal, shutdownChannel chan bool, prometheusListener *net.Listener) (exitCode int) {
	switch sig {
	case syscall.SIGTERM:
		logger.Infof("got sigterm, quiting gracefully")
		shutdownChannel <- true
		close(shutdownChannel)
		if prometheusListener != nil {
			(*prometheusListener).Close()
		}
		return (0)
	case syscall.SIGINT:
		fallthrough
	case os.Interrupt:
		logger.Infof("got sigint, quitting")
		shutdownChannel <- true
		close(shutdownChannel)
		if prometheusListener != nil {
			(*prometheusListener).Close()
		}
		return (1)
	case syscall.SIGHUP:
		logger.Infof("got sighup, reloading configuration...")
		if prometheusListener != nil {
			(*prometheusListener).Close()
		}
		return (-1)
	default:
		logger.Warnf("Signal not handled: %v", sig)
	}
	return 1
}

func setSysProcAttr(cmd *exec.Cmd) {
	// not supported on windows
}
