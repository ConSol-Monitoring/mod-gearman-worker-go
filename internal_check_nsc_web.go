package modgearman

import (
	"bytes"

	"github.com/consol-monitoring/check_nsc_web/pkg/checknscweb"
)

type InternalCheckNSCWeb struct{}

func (chk *InternalCheckNSCWeb) Check(output *bytes.Buffer, args []string) int {
	return checknscweb.Check(output, args)
}
