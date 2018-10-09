package main

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var run bool
var (
	workerCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "existing_workers",
		Help: "Currently existing Workers",
	})

	taskCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "completed_tasks",
		Help: "completed tasks since startup",
	})

	iddleWorkerCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "iddle_workers",
		Help: "workers waiting for new jobs",
	})

	workingWorkerCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "working_workers",
		Help: "Currently busy Workers",
	})

	userTimes = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "user_times_of_worker_jobs",
			Help:       "sum of the userTimes",
			Objectives: map[float64]float64{1: 0.01},
		},
		[]string{"description"})

	systemTimes = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "system_times_of_worker_jobs",
			Help:       "sum of the systemTimes ",
			Objectives: map[float64]float64{1: 0.01},
		},
		[]string{"description"})
)

func startPrometheus() {
	run = false
	if config.prometheus_server == "" {
		return
	}
	http.Handle("/metrics", promhttp.Handler())
	registerMetrics()

	logger.Error(http.ListenAndServe(config.prometheus_server, nil))

}

func registerMetrics() {

	//register the metrics
	if err := prometheus.Register(workerCount); err != nil {
		fmt.Println(err)
	}

	if err := prometheus.Register(taskCounter); err != nil {
		fmt.Println(err)
	}

	if err := prometheus.Register(iddleWorkerCount); err != nil {
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
