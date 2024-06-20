package modgearman

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
