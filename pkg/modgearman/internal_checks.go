package modgearman

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	time "time"
)

type InternalCheck interface {
	Check(ctx context.Context, output *bytes.Buffer, args []string, env []string) int
}

func execInternal(result *answer, cmd *command, received *request) {
	log.Tracef("using internal check for: %s", cmd.Command)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(received.timeout)*time.Second)
	defer cancel()

	go func() {
		defer cancel()
		defer logPanicInternalCheck(result)
		output := bytes.NewBuffer(nil)
		rc := cmd.InternalCheck.Check(ctx, output, cmd.Args, cmd.convertEnvToArray())
		result.output = output.String()
		result.returnCode = rc
	}()

	<-ctx.Done() // wait till command runs into timeout or is finished (canceled)
	ctxErr := ctx.Err()
	switch {
	case errors.Is(ctxErr, context.DeadlineExceeded):
		result.timedOut = true
	case errors.Is(ctxErr, context.Canceled):
	}
}

func logPanicInternalCheck(result *answer) {
	if rec := recover(); rec != nil {
		log.Errorf("********* PANIC (internal check) *********")
		log.Errorf("Panic: %s", rec)
		log.Errorf("**** Stack:")
		log.Errorf("%s", debug.Stack())
		log.Errorf("******************************************")

		result.output = fmt.Sprintf("UNKNOWN - internal check crashed: %s", rec)
		result.returnCode = stateUnknown
	}
}
