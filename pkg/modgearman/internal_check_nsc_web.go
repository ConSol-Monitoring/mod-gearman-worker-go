package modgearman

import (
	"bytes"
	"context"

	"github.com/consol-monitoring/check_snclient/pkg/checksnclient"
)

type InternalCheckSNClient struct{}

func (chk *InternalCheckSNClient) Check(ctx context.Context, output *bytes.Buffer, args, env []string) int {
	return checksnclient.Check(ctx, output, args, env)
}
