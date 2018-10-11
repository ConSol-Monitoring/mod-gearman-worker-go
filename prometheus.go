package main

import (
	"fmt"
	"net"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
)

var run bool
var (
	workerCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "modgearmanworker_workers_total",
		Help: "Total number of currently existing Workers",
	})

	idleWorkerCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "modgearmanworker_workers_idle",
		Help: "Total number of currently idling Workers",
	})

	workingWorkerCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "modgearmanworker_workers_busy",
		Help: "Total number of busy Workers",
	})

	taskCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "modgearmanworker_tasks_completed_total",
			Help: "total nuber of completed tasks since startup",
		},
		[]string{"type"})

	errorCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "modgearmanworker_tasks_errors_total",
			Help: "total nuber of errors in executed plugins (plugins with exit code > 0)",
		},
		[]string{"type"})

	userTimes = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "modgearmanworker_plugin_user_cpu_time_seconds",
			Help:       "sum of the userTimes of executed plugins",
			Objectives: map[float64]float64{1: 0.01},
		},
		[]string{"description"})

	systemTimes = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "modgearmanworker_plugin_system_cpu_time_seconds",
			Help:       "sum of the systemTimes of executed plugins",
			Objectives: map[float64]float64{1: 0.01},
		},
		[]string{"description"})
)

func startPrometheus(server string) (prometheusListener *net.Listener) {
	defer logPanicExit()
	if server != "" {
		l, err := net.Listen("tcp", server)
		prometheusListener = &l
		go func() {
			// make sure we log panics properly
			defer logPanicExit()

			if err != nil {
				logger.Fatalf("starting prometheus exporter failed: %s", err)
			}
			mux := http.NewServeMux()
			mux.Handle("/metrics", prometheus.Handler())
			http.Serve(l, mux)
		}()
		logger.Debugf("serving prometheus metrics at %s/metrics", server)
	}
	registerMetrics()
	return
}

var prometheusRegistered bool

func registerMetrics() {

	// registering twice will throw lots of errors
	if prometheusRegistered {
		return
	}
	prometheusRegistered = true

	//register the metrics
	if err := prometheus.Register(workerCount); err != nil {
		fmt.Println(err)
	}

	if err := prometheus.Register(taskCounter); err != nil {
		fmt.Println(err)
	}

	if err := prometheus.Register(errorCounter); err != nil {
		fmt.Println(err)
	}

	if err := prometheus.Register(idleWorkerCount); err != nil {
		fmt.Println(err)
	}
	if err := prometheus.Register(workingWorkerCount); err != nil {
		fmt.Println(err)
	}

	if err := prometheus.Register(userTimes); err != nil {
		fmt.Println(err)
	}

	if err := prometheus.Register(systemTimes); err != nil {
		fmt.Println(err)
	}

}
