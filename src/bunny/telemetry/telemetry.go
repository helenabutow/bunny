package telemetry

import (
	"bunny/config"
	"bunny/logging"
	"context"
	"log/slog"
	"os"
	"sync"

	kitlog "github.com/go-kit/log"
	client_golang_prometheus "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/tsdb"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
)

var logger *slog.Logger = nil
var ConfigUpdateChannel chan config.BunnyConfig = make(chan config.BunnyConfig, 1)
var OSSignalsChannel chan os.Signal = make(chan os.Signal, 1)
var exporter *prometheus.Exporter = nil
var provider *metric.MeterProvider = nil
var db *tsdb.DB = nil

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
	registry := client_golang_prometheus.NewRegistry()

	// Prometheus uses a logging library from outside the standard library
	// so we have to adapt it to work nicely with slog
	kitLogger := logging.NewSlogAdapterLogger()
	kitLogger = kitlog.With(kitLogger, "caller", kitlog.DefaultCaller)

	// TODO-MEDIUM: think about looking at not using the default options for tsdb
	// this help ensure that we don't use disk too much
	tsdb.Open(tsdbDirectoryPath, kitLogger, registry, tsdb.DefaultOptions(), tsdb.NewDBStats())

	// setup OpenTelemetry
	// the HTTP endpoint is in the ingress package
	// seems like an easy bit of memory and bandwidth to save
	exporter, err = prometheus.New(prometheus.WithoutScopeInfo(), prometheus.WithoutTargetInfo())
	if err != nil {
		logger.Error("error while creating Prometheus exporter", "err", err)
	}
	provider = metric.NewMeterProvider(metric.WithReader(exporter))
	// register a global default meter provider so that any libraries that we depend on have one to use
	otel.SetMeterProvider(provider)

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

// TODO-LOW: if we want to associate a trace with logs: https://github.com/go-slog/otelslog

// TODO-MEDIUM: a useful way to get traces to console (for debugging): https://github.com/equinix-labs/otel-cli#examples
