package modgearman

import (
	"fmt"
	"net"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

func startPrometheus(config *configurationStruct) (prometheusListener *net.Listener) {
	registerMetrics()
	build := ""
	if config.build != "" {
		build = fmt.Sprintf(":%s", config.build)
	}
	infoCount.WithLabelValues(fmt.Sprintf("%s%s", VERSION, build), config.identifier).Set(1)

	if config.prometheusServer == "" {
		return
	}

	l, err := net.Listen("tcp", config.prometheusServer)
	if err != nil {
		logger.Fatalf("starting prometheus exporter failed: %s", err)
	}
	prometheusListener = &l
	go func() {
		// make sure we log panics properly
		defer logPanicExit()
		mux := http.NewServeMux()
		handler := promhttp.InstrumentMetricHandler(
			prometheus.DefaultRegisterer, promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{EnableOpenMetrics: true}),
		)
		mux.Handle("/metrics", handler)
		http.Serve(l, mux)
		logger.Debugf("prometheus listener %s stopped", config.prometheusServer)
	}()
	logger.Debugf("serving prometheus metrics at %s/metrics", config.prometheusServer)
	return
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
		fmt.Println(err)
	}

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

	if err := prometheus.Register(ballooningWorkerCount); err != nil {
		fmt.Println(err)
	}

	if err := prometheus.Register(userTimes); err != nil {
		fmt.Println(err)
	}

	if err := prometheus.Register(systemTimes); err != nil {
		fmt.Println(err)
	}
}

func buildExecExemplarLabels(result *answer, received *receivedStruct, basename string) prometheus.Labels {
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

func updatePrometheusExecMetrics(config *configurationStruct, result *answer, received *receivedStruct, com *command) {
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
		errorCounter.WithLabelValues(received.typ, result.execType).(prometheus.ExemplarAdder).AddWithExemplar(1, exemplarLabels)
	}
}
