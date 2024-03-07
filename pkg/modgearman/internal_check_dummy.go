package modgearman

import (
	"bytes"
	"context"
	"fmt"
	"io"
)

type InternalCheckDummy struct{}

func (chk *InternalCheckDummy) Check(_ context.Context, output *bytes.Buffer, args []string) (rc int) {
	if len(args) == 0 {
		chk.printHelp(output)

		return 3
	}

	switch args[0] {
	case "0":
		rc = 0
		fmt.Fprintf(output, "OK:")
	case "1":
		rc = 1
		fmt.Fprintf(output, "WARNING:")
	case "2":
		rc = 2
		fmt.Fprintf(output, "CRITICAL:")
	case "3":
		rc = 3
		fmt.Fprintf(output, "UNKNOWN:")
	case "-h", "--help":
		chk.printHelp(output)

		return 3
	case "-v", "--version":
		chk.printVersion(output)

		return 3
	default:
		fmt.Fprintf(output, "UNKNOWN: Status %s is not a supported error state", args[0])

		return 3
	}

	if len(args) == 1 {
		return rc
	}

	fmt.Fprintf(output, " %s", args[1])

	return rc
}

func (chk *InternalCheckDummy) printHelp(output io.Writer) {
	chk.printVersion(output)
	fmt.Fprintf(output, `This plugin will simply return the state corresponding to the numeric value
of the <state> argument with optional text


Usage:
 check_dummy <integer state> [optional text]

Options:
 -h, --help
	Print detailed help screen
 -V, --version
	Print version information

`)
}

func (chk *InternalCheckDummy) printVersion(output io.Writer) {
	fmt.Fprintf(output, "check_dummy (internal mod-gearman-worker v%s)\n", VERSION)
}
