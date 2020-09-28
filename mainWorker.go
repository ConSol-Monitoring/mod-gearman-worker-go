package modgearman

import (
	"bufio"
	"net"
	"os"
	"strings"
	"sync"
	time "time"
)

const (
	DefaultConnectionTimeout = 30

	UtilizationWatermakeLow = 90
)

/*
* starts the min workers
* manages the worker list
* spawns new workers if needed
* kills worker being to old
 */

type mainWorker struct {
	activeWorkers int
	workerMap     map[string]*worker
	workerMapLock *sync.RWMutex
	statusWorker  *worker
	min1          float64
	min5          float64
	min15         float64
	config        *configurationStruct
	key           []byte
	tasks         int
	idleSince     time.Time
	serverStatus  map[string]string
	running       bool
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
	w.RetryFailedConnections()
	initialiseDupServerConsumers(w.config)
	return w
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
		if !w.idle {
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
	if !w.checkLoads() {
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

	// reduce workers at spawnrate
	for i := 0; i < w.config.spawnRate; i++ {
		if len(w.workerMap) <= w.config.minWorker {
			break
		}
		// stop first idle worker
		logger.Debugf("manageWorkers: stopping one...")
		for _, worker := range w.workerMap {
			if worker.idle {
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
func (w *mainWorker) getLoadAvg() {
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
}

// checks if all the loadlimits get checked, when values are set
func (w *mainWorker) checkLoads() bool {
	if w.config.loadLimit1 <= 0 && w.config.loadLimit5 <= 0 && w.config.loadLimit15 <= 0 {
		return true
	}

	w.getLoadAvg()
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
		_, err := net.DialTimeout("tcp", address, DefaultConnectionTimeout*time.Second)
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

// StopAllWorker stops all check worker and the status worker
func (w *mainWorker) StopAllWorker(state MainStateType) {
	w.workerMapLock.RLock()
	workerMap := w.workerMap
	workerNum := len(workerMap)
	w.workerMapLock.RUnlock()
	exited := make(chan int, 1)
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
	wait := 10 * time.Second
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
