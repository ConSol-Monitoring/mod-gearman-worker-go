package modgearman

import (
	"fmt"
	"github.com/appscode/g2/client"
	"os"
)

type CheckGmArgs struct {
	Usage          bool
	Verbose        bool
	Version        bool
	Timeout        int
	JobWarning     int
	JobCritical    int
	WorkerWarning  int
	WorkerCritical int
	Host           string
	TextToSend     string
	SendAsync      bool
	TextToExpect   string
	Queue          string
	UniqueId       string
	CritZeroWorker int
}

func Check_gearman(args CheckGmArgs) {
	if args.Host == "" {
		fmt.Fprintf(os.Stderr, "Error - np hostname given\n\n")
		printUsage()

		return
	}

	if args.TextToSend != "" && args.Queue == "" {
		fmt.Fprintf(os.Stderr, "Error - need queue (-q) when sending job\n\n")
		printUsage()

		return
	}
}

func checkWorker(queue string, textToSend string, textToExpect string) {
	uniqueJobId := textToSend
	if uniqueJobId == "" {
		uniqueJobId = "check"
	}

}

func createClient(server string) error {
	currClient, err := client.New("tcp", server)
	if err != nil {
		return fmt.Errorf("%s UNKNOWN - cannot create gearman client\n", "check_gearman")
	}

}
