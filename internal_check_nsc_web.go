package modgearman

import (
	"bytes"
	"context"

	"github.com/consol-monitoring/check_nsc_web/pkg/checknscweb"
)

type InternalCheckNSCWeb struct{}

func (chk *InternalCheckNSCWeb) Check(ctx context.Context, output *bytes.Buffer, args []string) int {
	return checknscweb.Check(ctx, output, args)
}
