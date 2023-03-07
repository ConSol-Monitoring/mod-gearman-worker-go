package modgearman

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

// NegateDefaultTimeout sets the default timeout if negate is used
const NegateDefaultTimeout = 11

type Negate struct {
	Timeout        int
	TimeoutResult  string
	OKStatus       string
	WarningStatus  string
	CriticalStatus string
	UnknownStatus  string
	Substitute     bool
}

func NewNegate() *Negate {
	return &Negate{
		Timeout:    NegateDefaultTimeout,
		Substitute: false,
	}
}

func (n *Negate) DefineFlags(fs *flag.FlagSet) {
	fs.IntVar(&n.Timeout, "timeout", n.Timeout, "Seconds before plugin times out")
	fs.IntVar(&n.Timeout, "t", n.Timeout, "Seconds before plugin times out")
	fs.StringVar(&n.TimeoutResult, "timeout-result", "", "Custom result on Negate timeouts")
	fs.StringVar(&n.TimeoutResult, "T", "", "Custom result on Negate timeouts")
	fs.StringVar(&n.OKStatus, "ok", "", "STATUS for OK result")
	fs.StringVar(&n.OKStatus, "o", "", "STATUS for OK result")
	fs.StringVar(&n.WarningStatus, "warning", "", "STATUS for WARNING result")
	fs.StringVar(&n.WarningStatus, "w", "", "STATUS for WARNING result")
	fs.StringVar(&n.CriticalStatus, "critical", "", "STATUS for CRITICAL result")
	fs.StringVar(&n.CriticalStatus, "c", "", "STATUS for CRITICAL result")
	fs.StringVar(&n.UnknownStatus, "unknown", "", "STATUS for UNKNOWN result")
	fs.StringVar(&n.UnknownStatus, "u", "", "STATUS for UNKNOWN result")
	fs.BoolVar(&n.Substitute, "substitute", false, "Substitute output text as well")
	fs.BoolVar(&n.Substitute, "s", false, "Substitute output text as well")
}

func (n *Negate) Parse(args []string) error {
	fs := flag.NewFlagSet("negate", flag.ContinueOnError)
	n.DefineFlags(fs)
	fs.SetOutput(io.Discard)

	err := fs.Parse(args)
	if err != nil {
		return fmt.Errorf("flags: %w: %s", err, err.Error())
	}

	// If nothing is specified, permutes OK and CRITICAL
	if n.OKStatus == "" && n.WarningStatus == "" && n.CriticalStatus == "" && n.UnknownStatus == "" {
		n.OKStatus = "CRITICAL"
		n.CriticalStatus = "OK"
	}
	n.OKStatus = n.ConvertStatusNumber(n.OKStatus)
	n.WarningStatus = n.ConvertStatusNumber(n.WarningStatus)
	n.CriticalStatus = n.ConvertStatusNumber(n.CriticalStatus)
	n.UnknownStatus = n.ConvertStatusNumber(n.UnknownStatus)
	n.TimeoutResult = n.ConvertStatusNumber(n.TimeoutResult)

	return nil
}

func (n *Negate) ConvertStatusNumber(arg string) string {
	switch arg {
	case "0":
		return "OK"
	case "1":
		return "WARNING"
	case "2":
		return "CRITICAL"
	case "3":
		return "UNKNOWN"
	default:
		return arg
	}
}

func (n *Negate) Status2Int(status string) int {
	switch status {
	case "OK":
		return 0
	case "WARNING":
		return 1
	case "CRITICAL":
		return 2
	case "UNKNOWN":
		return 2
	default:
		return 3
	}
}

func ParseNegate(com *command) {
	mainProgIndex := -1
	for i, arg := range com.Args {
		// main command must start with an /
		if strings.HasPrefix(arg, "/") {
			mainProgIndex = i
			break
		}
	}

	if mainProgIndex == -1 {
		logger.Debugf("cannot parse negate args, didn't find main program")
		return
	}

	negate := NewNegate()
	err := negate.Parse(com.Args[0:mainProgIndex])
	if err != nil {
		logger.Debugf("cannot parse negate args: %w: %s", err, err.Error())
		return
	}
	com.Command = com.Args[mainProgIndex]
	com.Args = com.Args[mainProgIndex+1:]
	com.Negate = negate
}

func (n *Negate) Apply(result *answer) {
	switch result.returnCode {
	case 0:
		n.ApplyNewCode(result, "OK", n.OKStatus)
	case 1:
		n.ApplyNewCode(result, "WARNING", n.WarningStatus)
	case 2:
		n.ApplyNewCode(result, "CRITICAL", n.CriticalStatus)
	case 3:
		n.ApplyNewCode(result, "UNKNOWN", n.UnknownStatus)
	}
}

func (n *Negate) ApplyNewCode(result *answer, from string, to string) {
	// no new value set at all
	if to == "" {
		return
	}
	result.returnCode = n.Status2Int(to)

	if !n.Substitute {
		return
	}
	result.output = strings.Replace(result.output, from, to, 1)
}

func (n *Negate) SetTimeoutReturnCode(result *answer) {
	if n.TimeoutResult == "" {
		return
	}
	result.returnCode = n.Status2Int(n.TimeoutResult)
}
