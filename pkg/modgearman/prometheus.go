package modgearman

import (
	"fmt"
	"net"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
)

var (
	infoCount = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "modgearmanworker_info",
			Help: "information about this worker",
		},
		[]string{"version", "identifier"})

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

	ballooningWorkerCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "modgearmanworker_workers_ballooning",
		Help: "Total number of extra ballooning Workers running",
	})

	taskCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "modgearmanworker_tasks_completed_total",
			Help: "total number of completed tasks since startup",
		},
		[]string{"type", "exec"})

	errorCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "modgearmanworker_tasks_errors_total",
			Help: "total number of errors in executed plugins (plugins with exit code > 0)",
		},
		[]string{"type", "exec"})

	userTimes = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "modgearmanworker_plugin_user_cpu_time_seconds",
			Help:       "sum of the userTimes of executed plugins",
			Objectives: map[float64]float64{1: 0.01},
		},
		[]string{"description", "exec"})

	systemTimes = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "modgearmanworker_plugin_system_cpu_time_seconds",
			Help:       "sum of the systemTimes of executed plugins",
			Objectives: map[float64]float64{1: 0.01},
		},
		[]string{"description", "exec"})
)

func startPrometheus(config *config) (prometheusListener net.Listener) {
	registerMetrics()
	build := ""
	if config.build != "" {
		build = fmt.Sprintf(":%s", config.build)
	}
	infoCount.WithLabelValues(fmt.Sprintf("%s%s", VERSION, build), config.identifier).Set(1)

	if config.prometheusServer == "" {
		return nil
	}

	listen, err := net.Listen("tcp", config.prometheusServer)
	if err != nil {
		log.Fatalf("starting prometheus exporter failed: %s", err)
	}
	prometheusListener = listen
	go func() {
		// make sure we log panics properly
		defer logPanicExit()
		mux := http.NewServeMux()
		handler := promhttp.InstrumentMetricHandler(
			prometheus.DefaultRegisterer,
			promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{EnableOpenMetrics: true}),
		)
		mux.Handle("/metrics", handler)
		logError(http.Serve(listen, mux))
		log.Debugf("prometheus listener %s stopped", config.prometheusServer)
	}()
	log.Debugf("serving prometheus metrics at %s/metrics", config.prometheusServer)

	return prometheusListener
}

var prometheusRegistered bool

func registerMetrics() {
	// registering twice will throw lots of errors
	if prometheusRegistered {
		return
	}
	prometheusRegistered = true

	// register the metrics
	if err := prometheus.Register(infoCount); err != nil {
		log.Errorf("prometheus register failed: %s", err.Error())
	}

	if err := prometheus.Register(workerCount); err != nil {
		log.Errorf("prometheus register failed: %s", err.Error())
	}

	if err := prometheus.Register(taskCounter); err != nil {
		log.Errorf("prometheus register failed: %s", err.Error())
	}

	if err := prometheus.Register(errorCounter); err != nil {
		log.Errorf("prometheus register failed: %s", err.Error())
	}

	if err := prometheus.Register(idleWorkerCount); err != nil {
		log.Errorf("prometheus register failed: %s", err.Error())
	}

	if err := prometheus.Register(workingWorkerCount); err != nil {
		log.Errorf("prometheus register failed: %s", err.Error())
	}

	if err := prometheus.Register(ballooningWorkerCount); err != nil {
		log.Errorf("prometheus register failed: %s", err.Error())
	}

	if err := prometheus.Register(userTimes); err != nil {
		log.Errorf("prometheus register failed: %s", err.Error())
	}

	if err := prometheus.Register(systemTimes); err != nil {
		log.Errorf("prometheus register failed: %s", err.Error())
	}
}

func buildExecExemplarLabels(result *answer, received *request, basename string) prometheus.Labels {
	// prometheus panics if exemplars are too long, so make sure basename is small enough
	if len(basename) > 30 {
		basename = basename[0:30]
	}
	label := prometheus.Labels{
		"basename":         basename,
		"rc":               fmt.Sprintf("%d", result.returnCode),
		"exec":             result.execType,
		"compile_duration": fmt.Sprintf("%.5f", result.compileDuration),
		"runtime_duration": fmt.Sprintf("%.5f", result.runUserDuration+result.runSysDuration),
		"type":             received.typ,
	}

	return label
}

func updatePrometheusExecMetrics(config *config, result *answer, received *request, com *command) {
	if config.prometheusServer == "" {
		return
	}

	basename := getCommandQualifier(com)

	if result.runUserDuration > 0 || result.runSysDuration > 0 {
		userTimes.WithLabelValues(basename, result.execType).Observe(result.runUserDuration)
		systemTimes.WithLabelValues(basename, result.execType).Observe(result.runSysDuration)
	}

	if result.returnCode > 0 {
		exemplarLabels := buildExecExemplarLabels(result, received, basename)
		if exemplarAdd, ok := errorCounter.WithLabelValues(received.typ, result.execType).(prometheus.ExemplarAdder); ok {
			exemplarAdd.AddWithExemplar(1, exemplarLabels)
		}
	}
}

func promCounterVecSum(counterVec *prometheus.CounterVec) (totalSum float64) {
	metrics := make(chan prometheus.Metric)

	// Run collection in a separate goroutine
	go func() {
		counterVec.Collect(metrics)
		close(metrics)
	}()

	for metric := range metrics {
		// Use a protobuf to store metric data
		metricProto := &dto.Metric{}
		err := metric.Write(metricProto)
		if err != nil {
			log.Warnf("Error writing metric: %s", err.Error())

			return
		}

		// Sum the counter value from each metric
		counter := metricProto.GetCounter()
		if counter != nil {
			totalSum += counter.GetValue()
		}
	}

	return totalSum
}
