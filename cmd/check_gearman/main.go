package main

import (
	"flag"
	"fmt"
	"github.com/consol-monitoring/mod-gearman-worker-go/pkg/modgearman"
	"os"
)

// Build contains the current git commit id
// compile passing -ldflags "-X main.Build <build sha1>" to set the id.
var Build string

const (
	defaultTimeout = 10
	defaultJobWarning
	defaultJobCritical    = 100
	defaultWorkerWarning  = 25
	defaultWorkerCritical = 50
	defaultCritZeroWorker = 1
)

func main() {
	args := modgearman.CheckGmArgs{}
	// Define a new FlagSet for avoiding collisions with other flags
	flagSet := flag.NewFlagSet("check_gearman", flag.ExitOnError)
	flagSet.Usage = func() { modgearman.PrintUsageCheckGearman(&args) }

	flagSet.StringVar(&args.Host, "H", "", "hostname")
	flagSet.IntVar(&args.Timeout, "t", defaultTimeout, "timeout in seconds")
	flagSet.IntVar(&args.JobWarning, "w", defaultJobWarning, "job warning level")
	flagSet.IntVar(&args.JobCritical, "c", defaultJobCritical, "job critical level")
	flagSet.BoolVar(&args.Verbose, "v", false, "verbose output")
	flagSet.BoolVar(&args.Version, "V", false, "print version")
	flagSet.IntVar(&args.WorkerWarning, "W", defaultWorkerWarning, "worker warning level")
	flagSet.IntVar(&args.WorkerCritical, "C", defaultWorkerCritical, "worker warning level")
	flagSet.StringVar(&args.TextToSend, "s", "", "text to send")
	flagSet.StringVar(&args.TextToExpect, "e", "", "text to expect")
	flagSet.BoolVar(&args.SendAsync, "a", false, "send async - will ignore")
	flagSet.StringVar(&args.Queue, "q", "", "queue")
	flagSet.StringVar(&args.UniqueID, "u", "", "unique job id")
	flagSet.IntVar(&args.CritZeroWorker, "x", defaultCritZeroWorker, "text to expect")

	// Parse the flags in the custom FlagSet
	err := flagSet.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags -> %s", err.Error())
		os.Exit(1)
	}

	modgearman.CheckGearman(&args)
}
