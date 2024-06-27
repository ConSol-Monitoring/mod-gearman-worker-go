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

func main() {
	args := modgearman.CheckGmArgs{}
	// Define a new FlagSet for avoiding collisions with other flags
	flagSet := flag.NewFlagSet("check_gearman", flag.ExitOnError)
	flagSet.Usage = func() { printUsage(args) }

	flagSet.StringVar(&args.Host, "H", "0.0.0.0", "hostname")
	flagSet.IntVar(&args.Timeout, "t", 10, "timeout in seconds")
	flagSet.IntVar(&args.JobWarning, "w", 10, "job warning level")
	flagSet.IntVar(&args.JobCritical, "c", 100, "job critical level")
	flagSet.BoolVar(&args.Verbose, "v", false, "verbose output")
	flagSet.BoolVar(&args.Version, "V", false, "print version")
	flagSet.IntVar(&args.WorkerWarning, "W", 25, "worker warning level")
	flagSet.IntVar(&args.WorkerCritical, "C", 50, "worker warning level")
	flagSet.StringVar(&args.TextToSend, "s", "", "text to send")
	flagSet.StringVar(&args.TextToExpect, "e", "", "text to expect")
	flagSet.BoolVar(&args.SendAsync, "a", true, "send async - will ignore")
	flagSet.StringVar(&args.Queue, "q", "", "queue")
	flagSet.StringVar(&args.UniqueId, "u", "", "unique job id")
	flagSet.IntVar(&args.CritZeroWorker, "x", 1, "text to expect")

	// Parse the flags in the custom FlagSet
	err := flagSet.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing flags -> %s", err.Error())
		os.Exit(1)
	}

	modgearman.Check_gearman(args)
}

func printUsage(args modgearman.CheckGmArgs) {
	fmt.Fprintf(os.Stdout, "usage:\n")
	fmt.Fprintf(os.Stdout, "\n")
	fmt.Fprintf(os.Stdout, "check_gearman [ -H=<hostname>[:port]         ]\n")
	fmt.Fprintf(os.Stdout, "              [ -t=<timeout>                 ]\n")
	fmt.Fprintf(os.Stdout, "              [ -w=<jobs warning level>      ]  default: %d\n", args.JobWarning)
	fmt.Fprintf(os.Stdout, "              [ -c=<jobs critical level>     ]  default: %d\n", args.JobCritical)
	fmt.Fprintf(os.Stdout, "              [ -W=<worker warning level>    ]  default: %d\n", args.WorkerWarning)
	fmt.Fprintf(os.Stdout, "              [ -C=<worker critical level>   ]  default: %d\n", args.WorkerCritical)
	fmt.Fprintf(os.Stdout, "              [ -q=<queue>                   ]\n")
	fmt.Fprintf(os.Stdout, "              [ -x=<crit on zero worker>     ]  default: %d\n", args.CritZeroWorker)
	fmt.Fprintf(os.Stdout, "\n")
	fmt.Fprintf(os.Stdout, "\n")
	fmt.Fprintf(os.Stdout, "to send a test job:\n")
	fmt.Fprintf(os.Stdout, "              [ -u=<unique job id>           ]  default: check\n")
	fmt.Fprintf(os.Stdout, "              [ -s=<send text>               ]\n")
	fmt.Fprintf(os.Stdout, "              [ -e=<expect text>             ]\n")
	fmt.Fprintf(os.Stdout, "              [ -a           send async      ]  will ignore -e\n")
	fmt.Fprintf(os.Stdout, "\n")
	fmt.Fprintf(os.Stdout, "              [ -h           print help      ]\n")
	fmt.Fprintf(os.Stdout, "              [ -v           verbose output  ]\n")
	fmt.Fprintf(os.Stdout, "              [ -V           print version   ]\n")
	fmt.Fprintf(os.Stdout, "\n")
	fmt.Fprintf(os.Stdout, " - You may set thresholds to 0 to disable them.\n")
	fmt.Fprintf(os.Stdout, " - You may use -x to enable critical exit if there is no worker for specified queue.\n")
	fmt.Fprintf(os.Stdout, " - Thresholds are only for server checks, worker checks are availability only\n")
	fmt.Fprintf(os.Stdout, "\n")
	fmt.Fprintf(os.Stdout, "perfdata format when checking job server:\n")
	fmt.Fprintf(os.Stdout, " 'queue waiting'=current waiting jobs;warn;crit;0 'queue running'=current running jobs 'queue worker'=current num worker;warn;crit;0\n")
	fmt.Fprintf(os.Stdout, "\n")
	fmt.Fprintf(os.Stdout, "Note: set your pnp RRD_STORAGE_TYPE to MULTIPLE to support changeing numbers of queues.\n")
	fmt.Fprintf(os.Stdout, "      see http://docs.pnp4nagios.org/de/pnp-0.6/tpl_custom for detailed information\n")
	fmt.Fprintf(os.Stdout, "\n")
	fmt.Fprintf(os.Stdout, "perfdata format when checking mod gearman worker:\n")
	fmt.Fprintf(os.Stdout, " worker=10 jobs=1508c\n")
	fmt.Fprintf(os.Stdout, "\n")
	fmt.Fprintf(os.Stdout, "Note: Job thresholds are per queue not totals.\n")
	fmt.Fprintf(os.Stdout, "\n")
	fmt.Fprintf(os.Stdout, "Examples:\n")
	fmt.Fprintf(os.Stdout, "\n")
	fmt.Fprintf(os.Stdout, "Check job server:\n")
	fmt.Fprintf(os.Stdout, "\n")
	fmt.Fprintf(os.Stdout, "%%>./check_gearman -H localhost -q host\n")
	fmt.Fprintf(os.Stdout, "check_gearman OK - 0 jobs running and 0 jobs waiting. Version: 0.14\n")
	fmt.Fprintf(os.Stdout, "\n")
	fmt.Fprintf(os.Stdout, "Check worker:\n")
	fmt.Fprintf(os.Stdout, "\n")
	fmt.Fprintf(os.Stdout, "%%> ./check_gearman -H <job server hostname> -q worker_<worker hostname> -t 10 -s check\n")
	fmt.Fprintf(os.Stdout, "check_gearman OK - host has 5 worker and is working on 0 jobs\n")
	fmt.Fprintf(os.Stdout, "%%> ./check_gearman -H <job server hostname> -q perfdata -t 10 -x\n")
	fmt.Fprintf(os.Stdout, "check_gearman CRITICAL - Queue perfdata has 155 jobs without any worker. |'perfdata_waiting'=155;10;100;0 'perfdata_running'=0 'perfdata_worker'=0;25;50;0\n")
	fmt.Fprintf(os.Stdout, "\n")
	fmt.Fprintf(os.Stdout, "Check result worker:\n")
	fmt.Fprintf(os.Stdout, "%%> ./check_gearman -H <job server hostname> -q check_results -t 10 -s check\n")
	fmt.Fprintf(os.Stdout, "OK - result worker running on host. Sending 14.9 jobs/s (avg duration:0.040ms). Version: 4.0.3|worker=3;;;0;3 avg_submit_duration=0.000040s;;;0;0.000429 jobs=2388c errors=0c\n")
	fmt.Fprintf(os.Stdout, "\n")
}
