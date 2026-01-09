package modgearman

import (
	"bytes"
	"context"

	"github.com/consol-monitoring/check_prometheus/pkg/checker"
)

type internalCheckPrometheus struct{}

func (chk *internalCheckPrometheus) Check(_ context.Context, output *bytes.Buffer, args []string) int {
	// args passed to this function does not have the executable as first element.
	// The cli parser library of check_prometheus however expects a program name
	// Just like a normal argc , argv invocation
	argsForCheck := make([]string, 1+len(args))

	argsForCheck[0] = "check_prometheus"
	for i, arg := range args {
		argsForCheck[i+1] = arg
	}

	state, msg, collection, _ := checker.Check(argsForCheck)

	stdout := checker.GenerateStdout(state, msg, collection)

	output.WriteString(stdout)

	return state.Code
}
