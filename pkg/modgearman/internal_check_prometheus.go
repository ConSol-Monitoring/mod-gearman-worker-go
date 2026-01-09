package modgearman

import (
	"bytes"
	"context"

	"github.com/consol-monitoring/check_prometheus/pkg/checker"
)

type internalCheckPrometheus struct{}

func (chk *internalCheckPrometheus) Check(ctx context.Context, output *bytes.Buffer, args []string) int {
	// args passed to this function does not have the executable at first.
	var argsForCheck []string = make([]string, 0)
	argsForCheck = append(argsForCheck, "check_prometheus")
	argsForCheck = append(argsForCheck, args...)

	state, msg, collection, _ := checker.Check(argsForCheck)

	stdout := checker.GenerateStdout(state, msg, collection)

	output.WriteString(stdout)

	return state.Code
}
