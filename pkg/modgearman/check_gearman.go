package modgearman

import (
	"fmt"
	"github.com/appscode/g2/client"
	"net"
	"os"
	"strings"
	"time"
)

const (
	stateOk       = 0
	stateWarning  = 1
	stateCritical = 2
	stateUnknown  = 3

	pluginName = "check_gearman"
	gmVersion  = "5.1.3"
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
	UniqueID       string
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

type responseData struct {
	statusCode int
	response   string
}

type checkGearmanIdGen struct {
}

func CheckGearman(args *CheckGmArgs) {
	if args.Version {
		PrintVersionCheckGearman()

		os.Exit(stateUnknown)
	}

	if args.Host == "" {
		fmt.Fprintf(os.Stderr, "Error - no hostname given\n\n")
		PrintUsageCheckGearman(args)

		return
	}

	if args.TextToSend != "" && args.Queue == "" {
		fmt.Fprintf(os.Stderr, "Error - need queue (-q) when sending job\n\n")
		PrintUsageCheckGearman(args)

		return
	}

	statusChan := make(chan int)

	go func() {
		if args.TextToSend != "" {
			// Using default global timeout instead on relying on library implementation of timeout
			statusChan <- checkWorker(args)
		} else {
			statusChan <- checkServer(args)
		}
	}()

	var statusCode int
	select {
	case statusCode = <-statusChan:
	case <-time.After(time.Duration(args.Timeout) * time.Second):
		fmt.Fprintf(os.Stderr, "%s CRITICAL - timed out\n", pluginName)
		statusCode = stateCritical
	}

	os.Exit(statusCode)
}

func checkWorker(args *CheckGmArgs) int {
	args.UniqueID = ternary(args.UniqueID == "", args.UniqueID, "check")

	res := responseData{
		statusCode: stateOk,
		response:   "",
	}

	err := createWorkerJob(args, &res)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s CRITICAL - job failed: %s\n", pluginName, err)

		return res.statusCode
	}

	if args.Verbose {
		fmt.Fprintf(os.Stdout, "%s\n", res.response)
	}

	if !args.SendAsync && args.TextToExpect != "" && res.response != "" {
		if strings.Contains(res.response, args.TextToExpect) {
			fmt.Fprintf(os.Stdout, "%s OK - send worker: '%s' response: '%s'\n",
				pluginName,
				args.TextToSend,
				res.response,
			)
		} else {
			fmt.Fprintf(os.Stdout, "%s CRITICAL - send worker: '%s' response: '%s', expected '%s'\n",
				pluginName,
				args.TextToSend,
				res.response,
				args.TextToExpect,
			)

			res.statusCode = stateCritical

			return res.statusCode
		}

		return res.statusCode
	}

	// If result starts with a number followed by a colon, use this as exit code
	if res.response != "" && len(res.response) > 1 && res.response[1] == ':' {
		res.statusCode = int(res.response[0] - '0')
		res.response = res.response[2:]
		fmt.Fprintf(os.Stdout, "%s\n", res.response)

		return res.statusCode
	}

	fmt.Fprintf(os.Stdout, "%s OK - %s\n", pluginName, res.response)

	return res.statusCode
}

func createWorkerJob(args *CheckGmArgs, res *responseData) (err error) {
	// Unique id for all tasks is just "check" because it's the main task performed and helps with performance in neamon
	client.IdGen = &checkGearmanIdGen{}

	if args.SendAsync {
		res.response = "sending background job succeeded"
		_, err = sendWorkerJobBg(args)
		if err != nil {
			res.statusCode = stateCritical

			return
		}
	} else {
		res.response, err = sendWorkerJob(args)
		if err != nil {
			res.statusCode = stateCritical

			return
		}
	}

	return
}

func (*checkGearmanIdGen) Id() string {
	return "check"
}

func checkServer(args *CheckGmArgs) (statusCode int) {
	queueList, version, err := getServerQueues(args.Host)
	if err != nil {
		statusCode = stateCritical
		fmt.Fprintf(os.Stderr, "%s\n", err)

		return
	}

	serverData := serverCheckData{
		RC:           stateOk,
		Message:      "",
		Checked:      0,
		TotalRunning: 0,
		TotalWaiting: 0,
		Version:      version,
	}

	statusCode = processServerData(queueList, &serverData, args)
	printData(&serverData, queueList, args)

	return
}

func getServerQueues(server string) ([]queue, string, error) {
	hostName := extractHostName(server)
	port, err := determinePort(server)
	if err != nil {
		return nil, "", err
	}
	serverAddress := fmt.Sprintf("%s:%d", hostName, port)

	connectionMap := map[string]net.Conn{}
	queueList, version, err := processGearmanQueues(serverAddress, connectionMap)
	if err != nil || len(queueList) == 0 {
		return queueList, "", err
	}

	return queueList, version, nil
}

func processServerData(queueList []queue, data *serverCheckData, args *CheckGmArgs) int {
	data.RC = stateOk

	for _, element := range queueList {
		if args.Queue != "" && args.Queue != element.Name {
			continue
		}
		data.Checked++
		data.TotalRunning += element.Running
		data.TotalWaiting += element.Waiting

		if element.Waiting > 0 && element.AvailWorker == 0 {
			data.RC = stateCritical
			data.Message = fmt.Sprintf("Queue %s has %d job%s without any worker. ",
				element.Name,
				element.Waiting,
				ternary(element.Waiting > 1, "s", ""),
			)
		} else if args.JobCritical > 0 && element.Waiting >= args.JobCritical {
			data.RC = stateCritical
			data.Message = fmt.Sprintf("Queue %s has %d waiting job%s. ",
				element.Name,
				element.Waiting,
				ternary(element.Waiting > 1, "s", ""),
			)
		} else if args.WorkerCritical > 0 && element.AvailWorker >= args.WorkerCritical {
			data.RC = stateCritical
			data.Message = fmt.Sprintf("Queue %s has %d worker. ",
				element.Name,
				element.AvailWorker,
			)
		} else if args.CritZeroWorker == 1 && element.AvailWorker == 0 {
			data.RC = stateCritical
			data.Message = fmt.Sprintf("Queue %s has no worker. ", element.Name)
		} else if args.JobWarning > 0 && element.Waiting >= args.JobWarning {
			data.RC = stateWarning
			data.Message = fmt.Sprintf("Queue %s has %d waiting job%s. ",
				element.Name,
				element.Waiting,
				ternary(element.Waiting > 1, "s", ""),
			)
		} else if args.WorkerWarning > 0 && element.AvailWorker >= args.WorkerWarning {
			data.RC = stateWarning
			data.Message = fmt.Sprintf("Queue %s has %d worker. ", element.Name, element.AvailWorker)
		}
	}

	if args.Queue == "" && data.Checked == 0 {
		data.RC = stateWarning
		data.Message = fmt.Sprintf("Queue %s not found", args.Queue)
	}

	return data.RC
}

func printData(data *serverCheckData, queueList []queue, args *CheckGmArgs) {
	fmt.Fprintf(os.Stdout, "%s ", pluginName)
	switch data.RC {
	case stateOk:
		fmt.Fprintf(os.Stdout, "OK - %d job%s running and %d job%s waiting. Version: %s",
			data.TotalRunning,
			ternary(data.TotalRunning == 1, "", "s"),
			data.TotalWaiting,
			ternary(data.TotalWaiting == 1, "", "s"),
			data.Version,
		)
	case stateWarning:
		fmt.Fprintf(os.Stdout, "WARNING - ")
	case stateCritical:
		fmt.Fprintf(os.Stdout, "CRITICAL - ")
	case stateUnknown:
		fmt.Fprintf(os.Stdout, "UNKNOWN - ")
	}
	fmt.Fprintf(os.Stdout, "%s", data.Message)

	// Print performance data
	if len(queueList) > 0 {
		fmt.Fprintf(os.Stdout, "|")
		for _, element := range queueList {
			if args.Queue != "" && args.Queue != element.Name {
				continue
			}
			fmt.Fprintf(os.Stdout, "'%s_waiting'=%d;%d;%d;0 '%s_running'=%d '%s_worker'=%d;%d;%d;0 ",
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

func PrintVersionCheckGearman() {
	fmt.Fprintf(os.Stdout, "check_german: gearman version %s\n", gmVersion)
}

func PrintUsageCheckGearman(args *CheckGmArgs) {
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

/* Helper function */
func ternary[T any](condition bool, trueVal, falseVal T) T {
	if condition {
		return trueVal
	}

	return falseVal
}
