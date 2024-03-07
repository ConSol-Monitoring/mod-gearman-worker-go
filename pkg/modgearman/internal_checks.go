package modgearman

import (
	"bytes"
	"context"
	"errors"
	time "time"
)

type InternalCheck interface {
	Check(ctx context.Context, output *bytes.Buffer, args []string) int
}

func execInternal(result *answer, cmd *command, received *request) {
	log.Tracef("using internal check for: %s", cmd.Command)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(received.timeout)*time.Second)
	defer cancel()

	go func() {
		defer logPanicExit()
		output := bytes.NewBuffer(nil)
		rc := cmd.InternalCheck.Check(ctx, output, cmd.Args)
		result.output = output.String()
		result.returnCode = rc
		cancel()
	}()

	<-ctx.Done() // wait till command runs into timeout or is finished (canceled)
	ctxErr := ctx.Err()
	switch {
	case errors.Is(ctxErr, context.DeadlineExceeded):
		result.timedOut = true
	case errors.Is(ctxErr, context.Canceled):
	}
}
