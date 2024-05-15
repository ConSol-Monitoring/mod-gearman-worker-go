package main

import (
	"flag"
	"fmt"
	"os"
	"pkg/modgearman"
)

// Build contains the current git commit id
// compile passing -ldflags "-X main.Build <build sha1>" to set the id.
var Build string

func main() {
	args := modgearman.Args{}
	// Define a new FlagSet for avoiding collions with other flags
	var fs = flag.NewFlagSet("gearman_top", flag.ExitOnError)

	fs.BoolVar(&args.Usage, "h", false, "Print usage")
	fs.BoolVar(&args.Version, "V", false, "Print version")
	fs.BoolVar(&args.Quiet, "q", false, "Quiet mode")
	fs.BoolVar(&args.Batch, "b", false, "Batch mode")
	fs.BoolVar(&args.Verbose, "v", false, "Verbose output")
	fs.Float64Var(&args.Interval, "i", 1.0, "Set interval")
	fs.Func("H", "Add host", modgearman.Add2HostList)

	// Parse the flags in the custom FlagSet
	err := fs.Parse(os.Args[1:])
	if err != nil {
		fmt.Println("Error parsing flags:", err)
		os.Exit(1)
	}

	// Call the GearmanTop function with the parsed arguments
	modgearman.GearmanTop(&args)
}
