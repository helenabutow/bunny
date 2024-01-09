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
	"github.com/go-logr/logr"
	client_golang_prometheus "github.com/prometheus/client_golang/prometheus"
	client_golang_prometheus_collectors "github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/tsdb"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	otel_not_sdk_metric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
)

var logger *slog.Logger = nil
var ConfigUpdateChannel chan config.BunnyConfig = make(chan config.BunnyConfig, 1)
var OSSignalsChannel chan os.Signal = make(chan os.Signal, 1)
var configStageChannels []chan config.ConfigStage = []chan config.ConfigStage{}
var telemetryConfig *config.TelemetryConfig = nil

// OpenTelemetry things
var meterProvider *metric.MeterProvider = nil
var traceProvider *trace.TracerProvider = nil

// Prometheus things
var promDB *tsdb.DB = nil
var PromRegistry *client_golang_prometheus.Registry = nil
var promQueryEngine *promql.Engine = nil

func AddChannelListener(configStageChannel *(chan config.ConfigStage)) {
	configStageChannels = append(configStageChannels, *configStageChannel)
}

func configureTelemetry() {
	logger.Info("configuring telemetry")
	var err error

	// setup Prometheus
	// TODO-LOW: doc this config and how memory backed emptyDirs or other fast storage should be used
	tsdbDirectoryPath := telemetryConfig.Prometheus.TSDBPath
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
	tsdbOptions := tsdb.DefaultOptions()
	tsdbOptions.RetentionDuration = int64(telemetryConfig.Prometheus.TSDBOptions.RetentionDurationMilliseconds)
	tsdbOptions.MinBlockDuration = int64(telemetryConfig.Prometheus.TSDBOptions.MinBlockDurationMilliseconds)
	tsdbOptions.MaxBlockDuration = int64(telemetryConfig.Prometheus.TSDBOptions.MaxBlockDurationMilliseconds)
	promDB, err = tsdb.Open(tsdbDirectoryPath, kitLogger, PromRegistry, tsdbOptions, tsdb.NewDBStats())
	if err != nil {
		logger.Error("error while creating Prometheus database", "err", err)
	}
	maxConcurrentQueries := telemetryConfig.Prometheus.PromQL.MaxConcurrentQueries
	activeQueryTracker := promql.NewActiveQueryTracker(tsdbDirectoryPath, maxConcurrentQueries, kitLogger)
	queryEngineOpts := promql.EngineOpts{
		Logger:             kitLogger,
		Reg:                PromRegistry,
		MaxSamples:         telemetryConfig.Prometheus.PromQL.EngineOptions.MaxSamples,
		Timeout:            time.Duration(telemetryConfig.Prometheus.PromQL.EngineOptions.TimeoutMilliseconds) * time.Millisecond,
		ActiveQueryTracker: activeQueryTracker,
		LookbackDelta:      time.Duration(telemetryConfig.Prometheus.PromQL.EngineOptions.LookbackDeltaMilliseconds) * time.Millisecond,
		NoStepSubqueryIntervalFn: func(rangeMillis int64) int64 {
			return time.Duration(time.Duration(telemetryConfig.Prometheus.PromQL.EngineOptions.NoStepSubqueryIntervalMilliseconds) * time.Millisecond).Milliseconds()
		},
		EnableAtModifier:     true,
		EnableNegativeOffset: true,
		EnablePerStepStats:   true,
	}
	promQueryEngine = promql.NewEngine(queryEngineOpts)

	// setup OpenTelemetry
	// TODO-HIGH: make this work because otel logs clash with our logs
	// make OpenTelemetry use our logger
	logrLogger := logr.FromSlogHandler(logger.Handler())
	otel.SetLogger(logrLogger)
	// setup the otel exporters
	var metricOptions []metric.Option = []metric.Option{}
	var traceProviderOptions []trace.TracerProviderOption = []trace.TracerProviderOption{}
	for _, exporterName := range telemetryConfig.OpenTelemetry.Exporters {
		switch exporterName {
		case "stdoutmetric":
			exporter, err := stdoutmetric.New(stdoutmetric.WithEncoder(logging.NewOtelEncoder(logger)))
			if err != nil {
				logger.Error("error while creating stdoutmetric exporter", "err", err)
				continue
			}
			var reader = metric.WithReader(metric.NewPeriodicReader(exporter, metric.WithInterval(1*time.Second)))
			metricOptions = append(metricOptions, reader)
		case "prometheus":
			// the HTTP Prometheus endpoints are in the ingress package
			// removing the scope and target info seems like an easy bit of memory and bandwidth to save
			exporter, err := prometheus.New(prometheus.WithoutScopeInfo(), prometheus.WithoutTargetInfo())
			if err != nil {
				logger.Error("error while creating prometheus exporter", "err", err)
				continue
			}
			var reader = metric.WithReader(exporter)
			metricOptions = append(metricOptions, reader)
		case "otlpmetrichttp":
			exporter, err := otlpmetrichttp.New(context.Background())
			if err != nil {
				logger.Error("error while creating otlpmetrichttp exporter", "err", err)
				continue
			}
			var reader = metric.WithReader(metric.NewPeriodicReader(exporter))
			metricOptions = append(metricOptions, reader)
		case "otlpmetricgrpc":
			exporter, err := otlpmetricgrpc.New(context.Background())
			if err != nil {
				logger.Error("error while creating otlpmetricgrpc exporter", "err", err)
				continue
			}
			var reader = metric.WithReader(metric.NewPeriodicReader(exporter))
			metricOptions = append(metricOptions, reader)
		case "stdouttrace":
			exporter, err := stdouttrace.New(stdouttrace.WithWriter(logging.NewOtelWriter(logger)))
			if err != nil {
				logger.Error("error while creating stdouttrace exporter", "err", err)
				continue
			}
			traceProviderOptions = append(traceProviderOptions, trace.WithBatcher(exporter))
		case "otlptracehttp":
			exporter, err := otlptracehttp.New(context.Background())
			if err != nil {
				logger.Error("error while creating otlptracehttp exporter", "err", err)
				continue
			}
			traceProviderOptions = append(traceProviderOptions, trace.WithBatcher(exporter))
		case "otlptracegrpc":
			exporter, err := otlptracegrpc.New(context.Background())
			if err != nil {
				logger.Error("error while creating otlptracegrpc exporter", "err", err)
				continue
			}
			traceProviderOptions = append(traceProviderOptions, trace.WithBatcher(exporter))
		}
	}
	// set the service name
	var serviceNameAttribute = attribute.String("service.name", "bunny")
	var serviceNameResource = resource.NewWithAttributes("", serviceNameAttribute)
	metricOptions = append(metricOptions, metric.WithResource(serviceNameResource))
	traceProviderOptions = append(traceProviderOptions, trace.WithResource(serviceNameResource))
	// make sure that traces are propagated with baggage
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	// create and register the providers
	meterProvider = metric.NewMeterProvider(metricOptions...)
	traceProvider = trace.NewTracerProvider(traceProviderOptions...)
	// register a global default providers so that any libraries that we depend on have one to use
	otel.SetMeterProvider(meterProvider)
	otel.SetTracerProvider(traceProvider)

	// notify of telemetry config completion via channel
	for _, configStageChannel := range configStageChannels {
		configStageChannel <- config.ConfigStageTelemetryCompleted
	}

	logger.Info("telemetry configured")
}

func GoTelemetry(wg *sync.WaitGroup) {
	defer wg.Done()

	logger = logging.ConfigureLogger("telemetry")
	logger.Info("Telemetry is go!")

	for {
		logger.Debug("waiting for config or signal")
		select {
		case bunnyConfig, ok := <-ConfigUpdateChannel:
			if !ok {
				logger.Error("could not process config from config update channel")
				continue
			}
			logger.Info("received config update")
			telemetryConfig = &bunnyConfig.Telemetry
			configureTelemetry()
			logger.Info("config update processing complete")

		case signal, ok := <-OSSignalsChannel:
			if !ok {
				logger.Error("could not process signal from signal channel")
			}
			logger.Info("received signal. Ending go routine.", "signal", signal)
			meterProvider.Shutdown(context.Background())
			traceProvider.Shutdown(context.Background())
			logger.Info("completed shutdowns. Returning from go routine")
			return
		}
	}
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
