package modgearman

import (
	"fmt"
	"net"
	"os"
)

const (
	STATE_OK       = 0
	STATE_WARNING  = 1
	STATE_CRITICAL = 2
	STATE_UNKNOWN  = 3

	PLUGIN_NAME = "check_gearman"
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

type serverCheckData struct {
	RC           int
	Message      string
	Checked      int
	TotalRunning int
	TotalWaiting int
	Version      string
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

	if args.TextToSend != "" {
		// Check Worker
	} else {
		checkServer(args)
	}

}

func checkServer(args CheckGmArgs) {
	queueList, version, err := getServerQueues(args.Host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s", err)

		return
	}

	serverData := serverCheckData{
		RC:           STATE_OK,
		Message:      "",
		Checked:      0,
		TotalRunning: 0,
		TotalWaiting: 0,
		Version:      version,
	}

	processServerData(queueList, &serverData, args)
	printData(&serverData, queueList, args)

}

func getServerQueues(server string) ([]queue, string, error) {
	var connectionMap map[string]net.Conn
	queueList, version, err := processGearmanQueues(server, connectionMap)
	if err != nil || len(queueList) == 0 {
		fmt.Errorf("error with server conection - %s", err)

		return queueList, "", err
	}

	return queueList, version, nil
}

func processServerData(queueList []queue, data *serverCheckData, args CheckGmArgs) {
	for _, element := range queueList {
		if args.Queue != "" && args.Queue != element.Name {
			continue
		}
		data.Checked++
		data.TotalRunning += element.Running
		data.TotalWaiting += element.Waiting

		if element.Waiting > 0 && element.AvailWorker == 0 {
			data.RC = STATE_CRITICAL
			data.Message = fmt.Sprintf("Queue %s has %d job%s without any worker. ", element.Name, element.Waiting, ternary(element.Waiting > 1, "s", ""))
		} else if args.JobCritical > 0 && element.Waiting >= args.JobCritical {
			data.RC = STATE_CRITICAL
			data.Message = fmt.Sprintf("Queue %s has %d waiting job%s. ", element.Name, element.Waiting, ternary(element.Waiting > 1, "s", ""))
		} else if args.WorkerCritical > 0 && element.AvailWorker >= args.WorkerCritical {
			data.RC = STATE_CRITICAL
			data.Message = fmt.Sprintf("Queue %s has %d worker. ", element.Name, element.AvailWorker)
		} else if args.CritZeroWorker == 1 && element.AvailWorker == 0 {
			data.RC = STATE_CRITICAL
			data.Message = fmt.Sprintf("Queue %s has no worker. ", element.Name)
		} else if args.JobWarning > 0 && element.Waiting >= args.JobWarning {
			data.RC = STATE_WARNING
			data.Message = fmt.Sprintf("Queue %s has %d waiting job%s. ", element.Name, element.Waiting, ternary(element.Waiting > 1, "s", ""))
		} else if args.WorkerWarning > 0 && element.AvailWorker >= args.WorkerWarning {
			data.RC = STATE_WARNING
			data.Message = fmt.Sprintf("Queue %s has %d worker. ", element.Name, element.AvailWorker)
		}
	}

	if args.Queue == "" && data.Checked == 0 {
		data.RC = STATE_WARNING
		data.Message = fmt.Sprintf("Queue %s not found", args.Queue)
	}
}

func printData(data *serverCheckData, queueList []queue, args CheckGmArgs) {
	fmt.Fprintf(os.Stdout, "%s ", PLUGIN_NAME)
	switch data.RC {
	case STATE_OK:
		fmt.Fprintf(os.Stdout, "OK - %i job%s running and %i job%s waiting. Version: %s", data.TotalRunning, ternary(data.TotalRunning == 1, "", "s"), data.TotalWaiting, ternary(data.TotalWaiting == 1, "", "s"), data.Version)
	case STATE_WARNING:
		fmt.Fprintf(os.Stdout, "WARNING - ")
	case STATE_CRITICAL:
		fmt.Fprintf(os.Stdout, "CRITICAL - ")
	case STATE_UNKNOWN:
		fmt.Fprintf(os.Stdout, "UNKNOWN - ")
	}
	fmt.Fprintf(os.Stdout, "%s", data.Message)

	// Print performance data
	if len(queueList) > 0 {
		for _, element := range queueList {
			if args.Queue != "" && args.Queue != element.Name {
				continue
			}
			fmt.Fprintf(os.Stdout, "'%s_waiting'=%d:%d:%d: 0 '%s_running'=%i '%s_worker'=%i;%i;%i;0 ",
				element.Name,
				element.Waiting,
				args.JobWarning,
				args.JobCritical,
				element.Name,
				element.Running,
				element.Name,
				element.AvailWorker,
				args.WorkerWarning,
				args.WorkerCritical,
			)
		}
	}

	fmt.Fprintf(os.Stdout, "\n")
}

/* Helper function */
func ternary[T any](condition bool, trueVal, falseVal T) T {
	if condition {
		return trueVal
	}
	return falseVal
}
