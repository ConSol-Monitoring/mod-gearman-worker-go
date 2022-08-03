package modgearman

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"
	hpprof "net/http/pprof"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	time "time"
)

const (
	// DefaultConnectionTimeout sets the default connection timeout for tcp connections
	DefaultConnectionTimeout = 30

	// UtilizationWatermakeLow sets the lower mark when deciding if worker should be reduced
	UtilizationWatermakeLow = 90
)

/*
* starts the min workers
* manages the worker list
* spawns new workers if needed
* kills worker being to old
 */

type mainWorker struct {
	activeWorkers      int
	workerUtilization  int
	workerMap          map[string]*worker
	workerMapLock      *sync.RWMutex
	statusWorker       *worker
	min1               float64
	min5               float64
	min15              float64
	memTotal           int
	memFree            int
	maxOpenFiles       uint64
	maxPossibleWorker  int
	curBalooningWorker int
	config             *configurationStruct
	key                []byte
	tasks              int
	idleSince          time.Time
	serverStatus       map[string]string
	running            bool
	cpuProfileHandler  *os.File
}

func newMainWorker(configuration *configurationStruct, key []byte, workerMap *map[string]*worker) *mainWorker {
	w := &mainWorker{
		activeWorkers: 0,
		key:           key,
		config:        configuration,
		workerMap:     *workerMap,
		workerMapLock: new(sync.RWMutex),
		idleSince:     time.Now(),
		serverStatus:  make(map[string]string),
	}
	w.InitDebugOptions()
	w.RetryFailedConnections()
	initializeResultServerConsumers(w.config)
	initializeDupServerConsumers(w.config)
	return w
}

func (w *mainWorker) InitDebugOptions() {
	if w.config.flagProfile != "" {
		if w.config.flagCPUProfile != "" || w.config.flagMemProfile != "" {
			fmt.Print("ERROR: either use --debug-profile or --cpu/memprofile, not both\n")
			os.Exit(ExitCodeError)
		}
		runtime.SetBlockProfileRate(BlockProfileRateInterval)
		runtime.SetMutexProfileFraction(BlockProfileRateInterval)
		go func() {
			// make sure we log panics properly
			defer logPanicExit()
			_ = hpprof.Handler("/debug/pprof/")
			err := http.ListenAndServe(w.config.flagProfile, http.DefaultServeMux)
			if err != nil {
				logger.Warnf("http.ListenAndServe finished with: %e", err)
			}
		}()

		logger.Warnf("pprof profiler listening at http://%s/debug/pprof/", w.config.flagProfile)
	}

	if w.config.flagCPUProfile != "" {
		runtime.SetBlockProfileRate(BlockProfileRateInterval)
		cpuProfileHandler, err := os.Create(w.config.flagCPUProfile)
		if err != nil {
			fmt.Printf("ERROR: could not create CPU profile: %s", err.Error())
			os.Exit(ExitCodeError)
		}
		if err := pprof.StartCPUProfile(cpuProfileHandler); err != nil {
			fmt.Printf("ERROR: could not start CPU profile: %s", err.Error())
			os.Exit(ExitCodeError)
		}
		w.cpuProfileHandler = cpuProfileHandler
	}
}

func (w *mainWorker) manageWorkers(initialStart int) {
	// if there are no servers, we cannot do anything
	if len(w.ActiveServerList()) == 0 {
		logger.Tracef("manageWorkers: no active servers available, retrying...")
		return
	}

	// start status worker
	if w.statusWorker == nil {
		w.statusWorker = newStatusWorker(w.config, w)
	}

	activeWorkers := 0
	totalWorker := len(w.workerMap)
	for _, w := range w.workerMap {
		if w.activeJobs > 0 {
			activeWorkers++
		}
	}
	w.activeWorkers = activeWorkers
	logger.Tracef("manageWorkers: total: %d, active: %d (min: %d, max: %d)", totalWorker, activeWorkers, w.config.minWorker, w.config.maxWorker)
	workerCount.Set(float64(totalWorker))
	workingWorkerCount.Set(float64(activeWorkers))
	idleWorkerCount.Set(float64(totalWorker - activeWorkers))

	// as long as there are to few workers start them without a limit
	minWorker := w.config.minWorker
	if initialStart > 0 {
		minWorker = initialStart
	}
	logger.Tracef("manageWorkers: total: %d, active: %d, minWorker: %d", totalWorker, activeWorkers, minWorker)
	for i := minWorker - len(w.workerMap); i > 0; i-- {
		logger.Tracef("manageWorkers: starting minworker: %d, %d", minWorker-len(w.workerMap), i)
		worker := newWorker("check", w.config, w)
		w.registerWorker(worker)
		w.idleSince = time.Now()
	}

	if totalWorker > 0 {
		w.workerUtilization = (activeWorkers * 100) / totalWorker
	} else {
		w.workerUtilization = 0
	}

	// check if we have too many workers
	w.adjustWorkerBottomLevel()

	// check if we need more workers
	w.adjustWorkerTopLevel()
}

// check if we need more workers and start new ones
func (w *mainWorker) adjustWorkerTopLevel() {
	// only if all are busy
	if w.activeWorkers < len(w.workerMap) {
		return
	}
	// do not exceed maxWorker level
	if len(w.workerMap) >= w.config.maxWorker {
		return
	}
	// check load levels
	w.updateLoadAvg()
	if !w.checkLoads() {
		return
	}

	// check memory levels
	w.updateMemInfo()
	if !w.checkMemory() {
		return
	}

	// start new workers at spawn speed
	for i := 0; i < w.config.spawnRate; i++ {
		if len(w.workerMap) >= w.config.maxWorker {
			break
		}
		logger.Debugf("manageWorkers: starting one...")
		worker := newWorker("check", w.config, w)
		w.registerWorker(worker)
		w.idleSince = time.Now()
	}
}

// check if we have too many workers (less than 90% UtilizationWatermakeLow) active and above minWorker)
func (w *mainWorker) adjustWorkerBottomLevel() {
	if len(w.workerMap) == 0 {
		return
	}
	// below minmum level
	if len(w.workerMap) <= w.config.minWorker {
		return
	}
	// above 90% (UtilizationWatermakeLow) utilization
	if (w.activeWorkers / len(w.workerMap) * 100) >= UtilizationWatermakeLow {
		return
	}
	// not idling long enough
	if time.Now().Unix()-w.idleSince.Unix() <= int64(w.config.idleTimeout) {
		return
	}

	// reduce workers at sinkrate
	sinkRate := w.config.sinkRate
	if sinkRate <= 0 {
		sinkRate = w.config.spawnRate
	}
	for i := 0; i < sinkRate; i++ {
		if len(w.workerMap) <= w.config.minWorker {
			break
		}
		// stop first idle worker
		logger.Debugf("manageWorkers: stopping one...")
		for _, worker := range w.workerMap {
			if worker.activeJobs == 0 {
				worker.Shutdown()
				break
			}
		}
	}
}

// applyConfigChanges reloads configuration and returns true if config has been reloaded and no restart is required
// returns false if mainloop needs to be restarted
func (w *mainWorker) applyConfigChanges() (restartRequired bool, config *configurationStruct) {
	config, err := initConfiguration("mod_gearman_worker", w.config.build, printUsage, checkForReasonableConfig)
	if err != nil {
		restartRequired = false
		logger.Errorf("cannot reload configuration: %s", err.Error())
		return
	}

	// restart prometheus if necessary
	if config.prometheusServer != w.config.prometheusServer {
		if prometheusListener != nil {
			(*prometheusListener).Close()
		}
		prometheusListener = startPrometheus(config)
	}

	// do we have to restart our worker routines?
	switch {
	case strings.Join(config.server, "\n") != strings.Join(w.config.server, "\n"):
		restartRequired = true
	case strings.Join(config.dupserver, "\n") != strings.Join(w.config.dupserver, "\n"):
		restartRequired = true
	case config.dupServerBacklogQueueSize != w.config.dupServerBacklogQueueSize:
		restartRequired = true
	case config.host != w.config.host:
		restartRequired = true
	case config.service != w.config.service:
		restartRequired = true
	case strings.Join(config.hostgroups, "\n") != strings.Join(w.config.hostgroups, "\n"):
		restartRequired = true
	case strings.Join(config.servicegroups, "\n") != strings.Join(w.config.servicegroups, "\n"):
		restartRequired = true
	case config.eventhandler != w.config.eventhandler:
		restartRequired = true
	case config.notifications != w.config.notifications:
		restartRequired = true
	}

	if !restartRequired {
		// reopen logfile
		createLogger(config)

		// recreate cipher
		key := getKey(config)
		myCipher = createCipher(key, config.encryption)
		w.config = config
		logger.Debugf("reloading configuration finished, no worker restart necessary")
	}

	return
}

// reads the avg loads from /procs/loadavg
func (w *mainWorker) updateLoadAvg() {
	if w.config.loadLimit1 <= 0 && w.config.loadLimit5 <= 0 && w.config.loadLimit15 <= 0 {
		return
	}
	file, err := os.Open("/proc/loadavg")
	if err != nil {
		return
	}
	scanner := bufio.NewScanner(file)
	// read first line:
	scanner.Scan()
	firstline := scanner.Text()
	values := strings.Fields(firstline)

	w.min1 = getFloat(values[0])
	w.min5 = getFloat(values[1])
	w.min15 = getFloat(values[2])
	file.Close()
}

// checks if all the loadlimits get checked, when values are set
func (w *mainWorker) checkLoads() bool {
	if w.config.loadLimit1 <= 0 && w.config.loadLimit5 <= 0 && w.config.loadLimit15 <= 0 {
		return true
	}

	if w.config.loadLimit1 > 0 && w.min1 > 0 && w.config.loadLimit1 < w.min1 {
		logger.Debugf("not starting any more worker, load1 is too high: %f > %f", w.min1, w.config.loadLimit1)
		return false
	}

	if w.config.loadLimit5 > 0 && w.min5 > 0 && w.config.loadLimit5 < w.min5 {
		logger.Debugf("not starting any more worker, load5 is too high: %f > %f", w.min5, w.config.loadLimit5)
		return false
	}

	if w.config.loadLimit15 > 0 && w.min15 > 0 && w.config.loadLimit15 < w.min15 {
		logger.Debugf("not starting any more worker, load15 is too high: %f > %f", w.min15, w.config.loadLimit15)
		return false
	}

	return true
}

// reads the total/free memory by parsing /proc/meminfo
func (w *mainWorker) updateMemInfo() {
	if w.config.memLimit <= 0 {
		return
	}
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return
	}
	scanner := bufio.NewScanner(file)
	n := 2 // number of lines we are interested in
	for scanner.Scan() && n > 0 {
		switch {
		case bytes.HasPrefix(scanner.Bytes(), []byte(`MemFree:`)):
			values := strings.Fields(scanner.Text())
			if len(values) >= 1 {
				w.memFree = getInt(values[1])
			}
		case bytes.HasPrefix(scanner.Bytes(), []byte(`MemTotal:`)):
			values := strings.Fields(scanner.Text())
			if len(values) >= 1 {
				w.memTotal = getInt(values[1])
			}
		default:
			continue
		}
		n--
	}
	file.Close()
}

// checks the memory threshold in percent
func (w *mainWorker) checkMemory() bool {
	if w.config.memLimit <= 0 {
		return true
	}

	if w.memTotal <= 0 {
		return true
	}

	usedPercent := 100 - (w.memFree*100)/w.memTotal
	if w.config.memLimit > 0 && w.config.memLimit > usedPercent {
		logger.Debugf("not starting any more worker, memory usage is too high: %d%% > %d%%", usedPercent, w.config.memLimit)
		return false
	}

	return true
}
func (w *mainWorker) unregisterWorker(worker *worker) {
	switch worker.what {
	case "check":
		w.workerMapLock.Lock()
		delete(w.workerMap, worker.id)
		w.workerMapLock.Unlock()
	case "status":
		w.statusWorker = nil
	}
}

func (w *mainWorker) registerWorker(worker *worker) {
	if worker == nil {
		return
	}
	switch worker.what {
	case "check":
		w.workerMapLock.Lock()
		w.workerMap[worker.id] = worker
		w.workerMapLock.Unlock()
	case "status":
		if w.statusWorker != nil {
			logger.Errorf("duplicate status worker started")
		}
		w.statusWorker = worker
	}
}

// RetryFailedConnections updates status of failed servers
// returns true if server list has changed
func (w *mainWorker) RetryFailedConnections() bool {
	changed := false
	for _, address := range w.config.server {
		w.workerMapLock.RLock()
		status, ok := w.serverStatus[address]
		w.workerMapLock.RUnlock()
		previous := true
		if !ok || status != "" {
			previous = false
		}
		c, err := net.DialTimeout("tcp", address, DefaultConnectionTimeout*time.Second)
		c.Close()
		w.workerMapLock.Lock()
		if err != nil {
			w.serverStatus[address] = err.Error()
			if previous {
				changed = true
			}
		} else {
			w.serverStatus[address] = ""
			if !previous {
				changed = true
			}
		}
		w.workerMapLock.Unlock()
	}
	return changed
}

// ActiveServerList returns list of active servers
func (w *mainWorker) ActiveServerList() (servers []string) {
	w.workerMapLock.RLock()
	defer w.workerMapLock.RUnlock()

	servers = make([]string, 0)
	for _, address := range w.config.server {
		if w.serverStatus[address] == "" {
			servers = append(servers, address)
		}
	}
	return
}

// GetServerStatus returns server status for given address
func (w *mainWorker) GetServerStatus(addr string) (err string) {
	w.workerMapLock.RLock()
	defer w.workerMapLock.RUnlock()

	err = w.serverStatus[addr]
	return err
}

// SetServerStatus sets server status for given address
func (w *mainWorker) SetServerStatus(addr, err string) {
	w.workerMapLock.Lock()
	defer w.workerMapLock.Unlock()
	w.serverStatus[addr] = err
}

// Shutdown stop main worker
func (w *mainWorker) Shutdown(exitState MainStateType) {
	w.running = false
	w.StopAllWorker(exitState)

	// wait 5 seconds for result queue to empty
	for x := 0; x <= 5; x++ {
		if len(resultServerQueue) == 0 {
			break
		}
		time.Sleep(1 * time.Second)
	}

	if w.cpuProfileHandler != nil {
		pprof.StopCPUProfile()
		w.cpuProfileHandler.Close()
		logger.Warnf("cpu profile written to: %s", w.config.flagCPUProfile)
	}
	terminateDupServerConsumers()
	terminateResultServerConsumers()
}

// StopAllWorker stops all check worker and the status worker
func (w *mainWorker) StopAllWorker(state MainStateType) {
	w.workerMapLock.RLock()
	workerMap := w.workerMap
	workerNum := len(workerMap)
	w.workerMapLock.RUnlock()

	if workerNum == 0 {
		return
	}

	exited := make(chan int, workerNum)
	for _, wo := range workerMap {
		logger.Tracef("worker removed...")
		go func(wo *worker, ch chan int) {
			defer logPanicExit()
			// this might take a while, because it waits for the current job to complete
			wo.Shutdown()
			ch <- 1
		}(wo, exited)
	}

	// do not wait on shutdown via sigint
	wait := 30 * time.Second
	if state == Shutdown {
		wait = 1 * time.Second
	}

	// wait to end all worker
	timeout := time.NewTimer(wait)
	alreadyExited := 0
	for alreadyExited < workerNum {
		select {
		case <-timeout.C:
			logger.Infof("%s timeout hit while waiting for all workers to stop, remaining: %d", wait, workerNum-alreadyExited)
			w.StopStatusWorker()
			return
		case <-exited:
			alreadyExited++
			logger.Tracef("worker %d/%d exited", alreadyExited, workerNum)
			if alreadyExited >= workerNum {
				timeout.Stop()
				w.StopStatusWorker()
				return
			}
		}
	}
}

// StopStatusWorker stops all check worker and the status worker
func (w *mainWorker) StopStatusWorker() {
	if w.statusWorker == nil {
		return
	}
	logger.Debugf("statusworker removed...")
	w.statusWorker.Shutdown()
	w.statusWorker = nil
}
