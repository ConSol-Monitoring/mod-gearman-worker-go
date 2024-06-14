package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/consol-monitoring/mod-gearman-worker-go/pkg/modgearman"
)

// Build contains the current git commit id
// compile passing -ldflags "-X main.Build <build sha1>" to set the id.
var Build string

func main() {
	args := modgearman.Args{}
	// Define a new FlagSet for avoiding collisions with other flags
	flagSet := flag.NewFlagSet("gearman_top", flag.ExitOnError)

	flagSet.BoolVar(&args.Usage, "h", false, "Print usage")
	flagSet.BoolVar(&args.Version, "V", false, "Print version")
	flagSet.BoolVar(&args.Quiet, "q", false, "Quiet mode")
	flagSet.BoolVar(&args.Batch, "b", false, "Batch mode")
	flagSet.BoolVar(&args.Verbose, "v", false, "Verbose output")
	flagSet.Float64Var(&args.Interval, "i", 1.0, "Set interval")
	flagSet.Func("H", "Add host", func(host string) error {
		return modgearman.Add2HostList(host, &args.Hosts)
	})

	// Parse the flags in the custom FlagSet
	err := flagSet.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags -> %s", err.Error())
		os.Exit(1)
	}

	// Call the GearmanTop function with the parsed arguments
	modgearman.GearmanTop(&args)
}
