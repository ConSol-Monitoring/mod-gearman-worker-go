package main

import (
	"flag"
	"pkg/modgearman"
)

// Build contains the current git commit id
// compile passing -ldflags "-X main.Build <build sha1>" to set the id.
var Build string

func main() {
	// process flags
	args := modgearman.Args{}

	flag.BoolVar(&args.H_usage, "h", false, "Print usage")
	flag.BoolVar(&args.V_version, "V", false, "Print version")
	flag.BoolVar(&args.Q_quiet, "q", false, "Quiet mode")
	flag.BoolVar(&args.B_batch, "b", false, "Batch mode")
	flag.BoolVar(&args.V_verbose, "ver", false, "Verbose output")
	flag.IntVar(&args.I_interval, "i", 1, "Set interval")
	flag.Func("H", "Add host", modgearman.Add2HostList)

	flag.Parse()

	//modgearman.Send2gearmandadmin("status\nversion\n", "127.0.0.1", 4730)
	//modgearman.PrintSatus()
	modgearman.GearmanTop(&args)
}
