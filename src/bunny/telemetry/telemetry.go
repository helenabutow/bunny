package telemetry

import (
	"bunny/config"
	"bunny/logging"
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	kitlog "github.com/go-kit/log"
	client_golang_prometheus "github.com/prometheus/client_golang/prometheus"
	client_golang_prometheus_collectors "github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/tsdb"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	otel_not_sdk_metric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric"
)

var logger *slog.Logger = nil
var ConfigUpdateChannel chan config.BunnyConfig = make(chan config.BunnyConfig, 1)
var OSSignalsChannel chan os.Signal = make(chan os.Signal, 1)

// OpenTelemetry things
var exporter *prometheus.Exporter = nil
var provider *metric.MeterProvider = nil

// Prometheus things
var promDB *tsdb.DB = nil
var PromRegistry *client_golang_prometheus.Registry = nil
var promQueryEngine *promql.Engine = nil

func Init(sharedLogger *slog.Logger) {
	logger = sharedLogger
	logger.Info("Telemetry initializing")
	var err error

	// setup Prometheus
	// TODO-LOW: doc this env var and how memory backed emptyDirs or other fast storage should be used
	tsdbDirectoryPath := os.Getenv("TSDB_PATH")
	if tsdbDirectoryPath == "" {
		tsdbDirectoryPath, err = os.MkdirTemp("/tmp", "bunny-tsdb-")
		if err != nil {
			logger.Error("couldn't create a temp dir for the tsdb", "err", err)
			panic(err)
		}
	}
	PromRegistry = client_golang_prometheus.NewRegistry()
	// add some additional metrics that are useful
	// the process collector only produces metrics on Linux machines with an accessible /proc filesystem
	// an example of what it gives:
	// process_cpu_seconds_total 0.02
	// process_max_fds 1.048576e+06
	// process_open_fds 13
	// process_resident_memory_bytes 3.0150656e+07
	// process_start_time_seconds 1.70450088028e+09
	// process_virtual_memory_bytes 1.282310144e+09
	// process_virtual_memory_max_bytes 1.8446744073709552e+19
	processCollectorOpts := client_golang_prometheus_collectors.ProcessCollectorOpts{
		PidFn:        nil,
		Namespace:    "",
		ReportErrors: false,
	}
	PromRegistry.MustRegister(
		client_golang_prometheus_collectors.NewGoCollector(),
		client_golang_prometheus_collectors.NewProcessCollector(processCollectorOpts),
	)
	// Prometheus uses a logging library from outside the standard library
	// so we have to adapt it to work nicely with slog
	kitLogger := logging.NewSlogAdapterLogger()
	kitLogger = kitlog.With(kitLogger, "caller", kitlog.DefaultCaller)
	// TODO-MEDIUM: think about looking at not using the default options for tsdb
	// this help ensure that we don't use disk too much
	tsdbOptions := tsdb.DefaultOptions()
	promDB, err = tsdb.Open(tsdbDirectoryPath, kitLogger, PromRegistry, tsdbOptions, tsdb.NewDBStats())
	if err != nil {
		logger.Error("error while creating Prometheus database", "err", err)
	}
	// TODO-LOW: we should make the max concurrent queries configurable (instead of just setting it to 1000)
	activeQueryTracker := promql.NewActiveQueryTracker(tsdbDirectoryPath, 1000, kitLogger)
	queryEngineOpts := promql.EngineOpts{
		Logger: kitLogger,
		Reg:    PromRegistry,
		// TODO-LOW: we should make MaxSamples configurable
		// higher values allow for more memory to be used
		// see: https://manpages.debian.org/unstable/prometheus/prometheus.1.en.html#query.max_samples=50000000
		MaxSamples:         50000000,
		Timeout:            time.Duration(30) * time.Second,
		ActiveQueryTracker: activeQueryTracker,
		LookbackDelta:      time.Duration(0) * time.Second,
		NoStepSubqueryIntervalFn: func(rangeMillis int64) int64 {
			return time.Duration(1 * time.Second).Milliseconds()
		},
		EnableAtModifier:     true,
		EnableNegativeOffset: true,
		EnablePerStepStats:   true,
	}
	promQueryEngine = promql.NewEngine(queryEngineOpts)

	// setup OpenTelemetry
	// the HTTP endpoint is in the ingress package
	// removing the scope and target info seems like an easy bit of memory and bandwidth to save
	exporter, err = prometheus.New(prometheus.WithoutScopeInfo(), prometheus.WithoutTargetInfo())

	if err != nil {
		logger.Error("error while creating Prometheus exporter", "err", err)
	}
	provider = metric.NewMeterProvider(metric.WithReader(exporter))
	// register a global default meter provider so that any libraries that we depend on have one to use
	otel.SetMeterProvider(provider)

	// TODO-LOW: add support for pushing metrics to OTLP endpoints
	// right now we're using otelcol as a separate process to scrape otel provided Prometheus metrics and pushing them into Grafana
	// we should be able to push them directly into an OTLP endpoint instead

	logger.Info("Telemetry is initialized")
}

func GoTelemetry(wg *sync.WaitGroup) {
	defer wg.Done()

	logger.Info("Telemetry is go!")

	logger.Info("waiting for signal")
	signal, ok := <-OSSignalsChannel
	if !ok {
		logger.Error("could not process signal from signal channel")
	}
	logger.Info("received signal", "signal", signal)
	provider.Shutdown(context.Background())
	logger.Info("ending go routine")
}

// TODO-MEDIUM: remove the duplication between this and RangeQuery()
func InstantQuery(timeout time.Duration, queryString string, instantTime time.Time) (bool, error) {
	logger.Debug("execing instant query",
		"timeout", timeout,
		"queryString", queryString,
		"instantTime", instantTime,
	)
	var err error
	var queryOpts promql.QueryOpts
	deadline, cancelFunc := context.WithDeadline(context.Background(), time.Now().Add(timeout))
	query, err := promQueryEngine.NewInstantQuery(deadline, promDB, queryOpts, queryString, instantTime)
	var queryLogArgs []any = []any{
		"err", err,
		"timeout", timeout,
		"queryString", queryString,
		"instantTime", instantTime,
	}
	if err != nil {
		cancelFunc()
		if err == context.DeadlineExceeded {
			logger.Error("query deadline exceeded", queryLogArgs...)
		} else {
			logger.Error("could not create query", queryLogArgs...)
		}
		return false, err
	}
	var result *promql.Result = query.Exec(deadline)
	defer query.Close()
	var resultLogArgs []any = []any{
		"result.Err", result.Err,
		"result.Value", result.Value,
		"result.Warnings", result.Warnings,
		"queryString", queryString,
		"instantTime", instantTime,
	}
	handledResult, handledErr := handleQueryResult(result, resultLogArgs)
	cancelFunc()
	return handledResult, handledErr
}

func RangeQuery(timeout time.Duration, queryString string, startTime time.Time, endTime time.Time, interval time.Duration) (bool, error) {
	logger.Debug("execing instant query",
		"timeout", timeout,
		"queryString", queryString,
		"startTime", startTime,
		"endTime", endTime,
		"interval", interval,
	)
	var err error
	var queryOpts promql.QueryOpts
	deadline, cancelFunc := context.WithDeadline(context.Background(), time.Now().Add(timeout))
	query, err := promQueryEngine.NewRangeQuery(deadline, promDB, queryOpts, queryString, startTime, endTime, interval)
	var queryLogArgs []any = []any{
		"err", err,
		"timeout", timeout,
		"queryString", queryString,
		"startTime", startTime,
		"endTime", endTime,
		"interval", interval,
	}
	if err != nil {
		cancelFunc()
		if err == context.DeadlineExceeded {
			logger.Error("query deadline exceeded", queryLogArgs...)
		} else {
			logger.Error("could not create query", queryLogArgs...)
		}
		return false, err
	}
	var result *promql.Result = query.Exec(deadline)
	defer query.Close()
	var resultLogArgs []any = []any{
		"result.Err", result.Err,
		"result.Value", result.Value,
		"result.Warnings", result.Warnings,
		"queryString", queryString,
		"startTime", startTime,
		"endTime", endTime,
		"interval", interval,
	}
	handledResult, handledErr := handleQueryResult(result, resultLogArgs)
	cancelFunc()
	return handledResult, handledErr
}

func handleQueryResult(result *promql.Result, logArgs []any) (bool, error) {
	if result.Err != nil {
		logger.Error("error while executing query", logArgs...)
		return false, result.Err
	}
	if len(result.Warnings) > 0 {
		logger.Warn("warnings while executing query", logArgs...)
	}

	// TODO-MEDIUM: need to write tests for each of the query types
	// see: https://gobyexample.com/testing-and-benchmarking
	// also: https://pkg.go.dev/testing#hdr-Fuzzing
	switch result.Value.Type() {
	case parser.ValueTypeScalar:
		value, err := result.Scalar()
		if err != nil {
			message := "error while converting to scalar"
			logger.Error(message, logArgs...)
			return false, errors.New(message)
		}
		if value.V == 1.0 {
			logger.Debug("query result is true", logArgs...)
			return true, nil
		} else {
			logger.Debug("query result is false", logArgs...)
			return false, nil
		}
	case parser.ValueTypeVector:
		value, err := result.Vector()
		if err != nil {
			message := "error while converting to vector"
			logger.Error(message, logArgs...)
			return false, errors.New(message)
		}
		for _, sample := range value {
			if sample.H == nil {
				if sample.F != 1.0 {
					logger.Debug("query result is false", logArgs...)
					return false, nil
				}
			} else {
				message := "histograms in vectors unsupported"
				logger.Error(message, logArgs...)
				return false, errors.New(message)
			}
		}
		logger.Debug("query result is true", logArgs...)
		return true, nil
	case parser.ValueTypeMatrix:
		value, err := result.Matrix()
		if err != nil {
			message := "error while converting to matrix"
			logger.Error(message, logArgs...)
			return false, errors.New(message)
		}
		for _, series := range value {
			for i := 0; i < len(series.Floats); i++ {
				var fpoint promql.FPoint = series.Floats[i]
				if fpoint.F != 1.0 {
					logger.Debug("query result is false", logArgs...)
					return false, nil
				}
			}
		}
		logger.Debug("query result is true", logArgs...)
		return true, nil
	case parser.ValueTypeString:
		var value string = result.String()
		var splitStrings []string = strings.Split(value, " ")
		switch splitStrings[0] {
		case "1", "1.0":
			logger.Debug("query result is true", logArgs...)
			return true, nil
		case "0", "0.0":
			logger.Debug("query result is false", logArgs...)
			return false, nil
		default:
			message := "query result returned value that is neither 1 nor 0"
			logger.Error(message, logArgs...)
			return false, errors.New(message)
		}
	default:
		message := "unknown type for result"
		logger.Error(message, logArgs...)
		return false, errors.New(message)
	}
}

func NewAttributes(extraLabels []config.ExtraLabelsConfig) otel_not_sdk_metric.MeasurementOption {
	attributesCopy := make([]attribute.KeyValue, len(extraLabels))
	for i, extraLabelConfig := range extraLabels {
		attributesCopy[i] = attribute.Key(extraLabelConfig.Name).String(extraLabelConfig.Value)
	}
	return otel_not_sdk_metric.WithAttributeSet(attribute.NewSet(attributesCopy...))
}

func NewLabels(extraLabels []config.ExtraLabelsConfig) client_golang_prometheus.Labels {
	var m map[string]string = map[string]string{}
	for _, extraLabelConfig := range extraLabels {
		m[extraLabelConfig.Name] = extraLabelConfig.Value
	}
	logger.Debug("new labels", "m", m)
	return m
}

// TODO-LOW: if we want to associate a trace with logs: https://github.com/go-slog/otelslog

// TODO-MEDIUM: a useful way to get traces to console (for debugging): https://github.com/equinix-labs/otel-cli#examples
