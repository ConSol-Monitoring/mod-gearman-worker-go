package modgearman

import (
	"strings"
	"sync/atomic"
)

func runTestCmd(conf *configurationStruct, args []string) (rc int, output string) {
	conf.enableEmbeddedPerl = true
	check := &receivedStruct{
		typ:                "service",
		hostName:           "test check from commandline",
		serviceDescription: "test",
		commandLine:        strings.Join(args, " "),
	}
	if len(args) == 0 {
		return 3, "usage: mod_gearman_worker testcmd <cmd> <args>"
	}
	logger.Debugf("test cmd: %s\n", check.commandLine)
	command := parseCommand(check.commandLine, conf)
	if command.ExecType == EPN {
		startEmbeddedPerl(conf)
		atomic.StoreInt64(&aIsRunning, 1)
		defer stopAllEmbeddedPerl()
	}
	res := readAndExecute(check, conf)
	rc = res.returnCode
	output = res.output
	logger.Debugf("test cmd rc: %d\n", rc)
	return
}
