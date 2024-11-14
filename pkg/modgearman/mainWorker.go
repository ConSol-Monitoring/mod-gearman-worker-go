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
	"sync/atomic"
	time "time"
)

const (
	// DefaultConnectionTimeout sets the default connection timeout for tcp connections
	DefaultConnectionTimeout = 30

	// UtilizationWatermarkLow sets the lower mark when deciding if worker should be reduced
	UtilizationWatermarkLow = 90
)

/*
* starts the min workers
* manages the worker list
* spawns new workers if needed
* kills worker being to old
 */

type mainWorker struct {
	activeWorkers       int
	workerUtilization   int
	workerMap           map[string]*worker
	workerMapLock       *sync.RWMutex
	statusWorker        *worker
	min1                float64
	min5                float64
	min15               float64
	memTotal            uint64
	memFree             uint64
	maxOpenFiles        uint64
	maxPossibleWorker   int
	curBallooningWorker int
	cfg                 *config
	key                 []byte
	tasks               int
	idleSince           time.Time
	serverStatus        map[string]string
	running             bool
	cpuProfileHandler   *os.File
}

func newMainWorker(configuration *config, key []byte, workerMap map[string]*worker) *mainWorker {
	wrk := &mainWorker{
		activeWorkers: 0,
		key:           key,
		cfg:           configuration,
		workerMap:     workerMap,
		workerMapLock: new(sync.RWMutex),
		idleSince:     time.Now(),
		serverStatus:  make(map[string]string),
	}
	atomic.StoreInt64(&aIsRunning, 1)
	wrk.InitDebugOptions()
	wrk.RetryFailedConnections()
	initializeResultServerConsumers(wrk.cfg)
	initializeDupServerConsumers(wrk.cfg)

	return wrk
}

func (w *mainWorker) InitDebugOptions() {
	if w.cfg.flagProfile != "" {
		if w.cfg.flagCPUProfile != "" || w.cfg.flagMemProfile != "" {
			log.Errorf("ERROR: either use --debug-profile or --cpu/memprofile, not both")
			cleanExit(ExitCodeError)
		}
		runtime.SetBlockProfileRate(BlockProfileRateInterval)
		runtime.SetMutexProfileFraction(BlockProfileRateInterval)
		go func() {
			// make sure we log panics properly
			defer logPanicExit()
			_ = hpprof.Handler("/debug/pprof/")
			err := http.ListenAndServe(w.cfg.flagProfile, http.DefaultServeMux)
			if err != nil {
				log.Warnf("http.ListenAndServe finished with: %e", err)
			}
		}()

		log.Warnf("pprof profiler listening at http://%s/debug/pprof/", w.cfg.flagProfile)
	}

	if w.cfg.flagCPUProfile != "" {
		runtime.SetBlockProfileRate(BlockProfileRateInterval)
		cpuProfileHandler, err := os.Create(w.cfg.flagCPUProfile)
		if err != nil {
			log.Errorf("ERROR: could not create CPU profile: %s", err.Error())
			cleanExit(ExitCodeError)
		}
		if err := pprof.StartCPUProfile(cpuProfileHandler); err != nil {
			log.Errorf("ERROR: could not start CPU profile: %s", err.Error())
			cleanExit(ExitCodeError)
		}
		w.cpuProfileHandler = cpuProfileHandler
	}
}

func (w *mainWorker) manageWorkers(initialStart int) (reason string) {
	// if there are no servers, we cannot do anything
	if len(w.ActiveServerList()) == 0 {
		log.Tracef("manageWorkers: no active servers available, retrying...")

		return ""
	}

	// start status worker
	if w.statusWorker == nil {
		w.statusWorker = newStatusWorker(w.cfg, w)
	}

	activeWorkers := 0
	totalWorker := len(w.workerMap)
	for _, w := range w.workerMap {
		if w.activeJobs > 0 {
			activeWorkers++
		}
	}
	w.activeWorkers = activeWorkers
	log.Tracef("manageWorkers: total: %d, active: %d (min: %d, max: %d)",
		totalWorker, activeWorkers, w.cfg.minWorker, w.cfg.maxWorker)
	workerCount.Set(float64(totalWorker))
	workingWorkerCount.Set(float64(activeWorkers))
	idleWorkerCount.Set(float64(totalWorker - activeWorkers))

	// as long as there are to few workers start them without a limit
	minWorker := w.cfg.minWorker
	if initialStart > 0 {
		minWorker = initialStart
	}
	log.Tracef("manageWorkers: total: %d, active: %d, minWorker: %d", totalWorker, activeWorkers, minWorker)
	for i := minWorker - len(w.workerMap); i > 0; i-- {
		log.Tracef("manageWorkers: starting minworker: %d, %d", minWorker-len(w.workerMap), i)
		worker := newWorker("check", w.cfg, w)
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
	reason = w.adjustWorkerTopLevel()

	newTotalWorker := len(w.workerMap)
	if newTotalWorker != totalWorker {
		log.Debugf("adjusted workers: %d (utilization: %d%%)", newTotalWorker, w.workerUtilization)
	}

	return reason
}

// check if we need more workers and start new ones
func (w *mainWorker) adjustWorkerTopLevel() (failreason string) {
	// only if all are busy
	if w.activeWorkers < len(w.workerMap) {
		return ""
	}
	// do not exceed maxWorker level
	if len(w.workerMap) >= w.cfg.maxWorker {
		return ""
	}
	// check load levels
	w.updateLoadAvg()
	passed, failreason := w.checkLoads()
	if !passed {
		return failreason
	}

	// check memory levels
	w.updateMemInfo()
	passed, failreason = w.checkMemory()
	if !passed {
		return failreason
	}

	// start new workers at spawn speed
	for range w.cfg.spawnRate {
		if len(w.workerMap) >= w.cfg.maxWorker {
			break
		}
		log.Tracef("manageWorkers: starting one...")
		worker := newWorker("check", w.cfg, w)
		w.registerWorker(worker)
		w.idleSince = time.Now()
	}

	return ""
}

// check if we have too many workers (less than 90% UtilizationWatermarkLow) active and above minWorker)
func (w *mainWorker) adjustWorkerBottomLevel() {
	if len(w.workerMap) == 0 {
		return
	}
	// below minimum level
	if len(w.workerMap) <= w.cfg.minWorker {
		return
	}
	// above 90% (UtilizationWatermarkLow) utilization
	if (w.activeWorkers / len(w.workerMap) * 100) >= UtilizationWatermarkLow {
		return
	}
	// not idling long enough
	if time.Now().Unix()-w.idleSince.Unix() <= int64(w.cfg.idleTimeout) {
		return
	}

	// reduce workers at sinkrate
	sinkRate := w.cfg.sinkRate
	if sinkRate <= 0 {
		sinkRate = w.cfg.spawnRate
	}
	for range sinkRate {
		if len(w.workerMap) <= w.cfg.minWorker {
			break
		}
		// stop first idle worker
		log.Debugf("manageWorkers: stopping one...")
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
func (w *mainWorker) applyConfigChanges() (restartRequired bool, cfg *config) {
	cfg, err := initConfiguration("mod_gearman_worker", w.cfg.build, printUsage, checkForReasonableConfig)
	if err != nil {
		restartRequired = false
		log.Errorf("cannot reload configuration: %s", err.Error())

		return restartRequired, cfg
	}

	// restart prometheus if necessary
	if cfg.prometheusServer != w.cfg.prometheusServer {
		if prometheusListener != nil {
			prometheusListener.Close()
		}
		prometheusListener = startPrometheus(cfg)
	}

	// restart epn worker if necessary
	switch {
	case cfg.enableEmbeddedPerl != w.cfg.enableEmbeddedPerl,
		cfg.usePerlCache != w.cfg.usePerlCache,
		cfg.debug != w.cfg.debug:
		if ePNServer != nil {
			ePNServer.Stop(ePNGraceDelay)
			ePNServer = nil
		}
		startEmbeddedPerl(cfg)
	}

	// do we have to restart our worker routines?
	switch {
	case strings.Join(cfg.server, "\n") != strings.Join(w.cfg.server, "\n"):
		restartRequired = true
	case strings.Join(cfg.dupserver, "\n") != strings.Join(w.cfg.dupserver, "\n"):
		restartRequired = true
	case cfg.dupServerBacklogQueueSize != w.cfg.dupServerBacklogQueueSize:
		restartRequired = true
	case cfg.host != w.cfg.host:
		restartRequired = true
	case cfg.service != w.cfg.service:
		restartRequired = true
	case strings.Join(cfg.hostgroups, "\n") != strings.Join(w.cfg.hostgroups, "\n"):
		restartRequired = true
	case strings.Join(cfg.servicegroups, "\n") != strings.Join(w.cfg.servicegroups, "\n"):
		restartRequired = true
	case cfg.eventhandler != w.cfg.eventhandler:
		restartRequired = true
	case cfg.notifications != w.cfg.notifications:
		restartRequired = true
	}

	if !restartRequired {
		// reopen logfile
		createLogger(cfg)

		// recreate cipher
		key := getKey(cfg)
		myCipher = createCipher(key, cfg.encryption)
		w.cfg = cfg
		log.Debugf("reloading configuration finished, no worker restart necessary")
	}

	return restartRequired, cfg
}

// reads the avg loads from /procs/loadavg
func (w *mainWorker) updateLoadAvg() {
	if w.cfg.loadLimit1 <= 0 && w.cfg.loadLimit5 <= 0 && w.cfg.loadLimit15 <= 0 {
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

// checks all the load limits, if values are set
func (w *mainWorker) checkLoads() (ok bool, reason string) {
	if w.cfg.loadLimit1 <= 0 && w.cfg.loadLimit5 <= 0 && w.cfg.loadLimit15 <= 0 {
		return true, ""
	}

	if w.cfg.loadLimit1 > 0 && w.min1 > 0 && w.cfg.loadLimit1 < w.min1 {
		reason = fmt.Sprintf("cannot start any more worker, load1 is too high: %f > %f", w.min1, w.cfg.loadLimit1)
		log.Debug(reason)

		return false, reason
	}

	if w.cfg.loadLimit5 > 0 && w.min5 > 0 && w.cfg.loadLimit5 < w.min5 {
		reason = fmt.Sprintf("cannot start any more worker, load5 is too high: %f > %f", w.min5, w.cfg.loadLimit5)
		log.Debug(reason)

		return false, reason
	}

	if w.cfg.loadLimit15 > 0 && w.min15 > 0 && w.cfg.loadLimit15 < w.min15 {
		reason = fmt.Sprintf("cannot start any more worker, load15 is too high: %f > %f", w.min15, w.cfg.loadLimit15)
		log.Debug(reason)

		return false, reason
	}

	return true, ""
}

// reads the total/free memory by parsing /proc/meminfo
func (w *mainWorker) updateMemInfo() {
	if w.cfg.memLimit <= 0 {
		return
	}
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return
	}
	scanner := bufio.NewScanner(file)
	numLines := 3 // number of lines we are interested in
	w.memFree = 0
	w.memTotal = 0
	for scanner.Scan() && numLines > 0 {
		switch {
		// use MemFree as fallback, MemAvailable is the value we want to use
		case bytes.HasPrefix(scanner.Bytes(), []byte(`MemFree:`)):
			values := strings.Fields(scanner.Text())
			if len(values) >= 1 && w.memFree == 0 {
				w.memFree = uint64(getFloat(values[1]))
			}
		case bytes.HasPrefix(scanner.Bytes(), []byte(`MemAvailable:`)):
			values := strings.Fields(scanner.Text())
			if len(values) >= 1 {
				w.memFree = uint64(getFloat(values[1]))
			}
		case bytes.HasPrefix(scanner.Bytes(), []byte(`MemTotal:`)):
			values := strings.Fields(scanner.Text())
			if len(values) >= 1 {
				w.memTotal = uint64(getFloat(values[1]))
			}
		default:
			continue
		}
		numLines--
	}
	file.Close()
}

// checks the memory threshold in percent
func (w *mainWorker) checkMemory() (ok bool, reason string) {
	if w.cfg.memLimit <= 0 {
		return true, ""
	}

	if w.memTotal <= 0 {
		return true, ""
	}

	usedPercent := 100 - (w.memFree*100)/w.memTotal
	if w.cfg.memLimit > 0 && usedPercent >= w.cfg.memLimit {
		reason := fmt.Sprintf("cannot start any more worker, memory usage is too high: %d%% > %d%% (free: %s)",
			usedPercent,
			w.cfg.memLimit,
			bytes2Human(w.memFree*1024),
		)
		log.Debug(reason)

		return false, reason
	}

	return true, ""
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
			log.Errorf("duplicate status worker started")
		}
		w.statusWorker = worker
	}
}

// RetryFailedConnections updates status of failed servers
// returns true if server list has changed
func (w *mainWorker) RetryFailedConnections() bool {
	changed := false
	for _, address := range w.cfg.server {
		w.workerMapLock.RLock()
		status, ok := w.serverStatus[address]
		w.workerMapLock.RUnlock()
		previous := true
		if !ok || status != "" {
			previous = false
		}
		con, err := net.DialTimeout("tcp", address, DefaultConnectionTimeout*time.Second)
		w.workerMapLock.Lock()
		if err != nil {
			w.serverStatus[address] = err.Error()
			if previous {
				changed = true
			}
		} else {
			con.Close()
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
	for _, address := range w.cfg.server {
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
		log.Warnf("cpu profile written to: %s", w.cfg.flagCPUProfile)
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
	for _, wrk := range workerMap {
		log.Tracef("worker removed...")
		go func(wo *worker, ch chan int) {
			defer logPanicExit()
			// this might take a while, because it waits for the current job to complete
			wo.Shutdown()
			ch <- 1
		}(wrk, exited)
	}

	// do not wait on shutdown via sigint
	wait := 5 * time.Second
	if state == Shutdown {
		wait = 1 * time.Second
	}

	// wait to end all worker
	timeout := time.NewTimer(wait)
	alreadyExited := 0
	for alreadyExited < workerNum {
		select {
		case <-timeout.C:
			log.Infof("%s timeout hit while waiting for all workers to stop, remaining: %d", wait, workerNum-alreadyExited)
			w.StopStatusWorker()
			// cancel remaining worker
			w.workerMapLock.RLock()
			workerMap = w.workerMap
			w.workerMapLock.RUnlock()
			for _, wo := range workerMap {
				wo.Cancel()
			}

			return
		case <-exited:
			alreadyExited++
			log.Tracef("worker %d/%d exited", alreadyExited, workerNum)
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
	log.Debugf("statusworker removed...")
	w.statusWorker.Shutdown()
	w.statusWorker = nil
}

// convert bytes into a human readable string
func bytes2Human(num uint64) string {
	const unit = 1024
	if num < unit {
		return fmt.Sprintf("%d B", num)
	}
	div, exp := unit, 0
	for n := num / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.2f %cB", float64(num)/float64(div), "KMGTPE"[exp])
}
