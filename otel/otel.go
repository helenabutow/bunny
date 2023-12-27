package otel

import (
	"bunny/config"
	"context"
	"log/slog"
	"os"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	api "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric"
)

var logger *slog.Logger = nil
var ConfigUpdateChannel chan config.BunnyConfig = make(chan config.BunnyConfig, 1)
var OSSignalsChannel chan os.Signal = make(chan os.Signal, 1)
var exporter *prometheus.Exporter = nil
var provider *metric.MeterProvider = nil
var Meter *api.Meter = nil

func Init(sharedLogger *slog.Logger) {
	logger = sharedLogger
	logger.Info("OTel initializing")

	// setup Prometheus
	// the HTTP endpoint is in the ingress package
	var err error
	// seems like an easy bit of memory and bandwidth to save
	exporter, err = prometheus.New(prometheus.WithoutScopeInfo(), prometheus.WithoutTargetInfo())
	if err != nil {
		logger.Error("error while creating Prometheus exporter", "err", err)
	}
	provider = metric.NewMeterProvider(metric.WithReader(exporter))

	// ick
	newMeter := provider.Meter("")
	Meter = &(newMeter)

	// register a global default meter provider so that any libraries that we depend on have one to use
	otel.SetMeterProvider(provider)

	logger.Info("OTel is initialized")
}

func GoOTel(wg *sync.WaitGroup) {
	defer wg.Done()

	logger.Info("OTel is go!")

	logger.Info("waiting for signal")
	signal, ok := <-OSSignalsChannel
	if !ok {
		logger.Error("could not process signal from signal channel")
	}
	logger.Info("received signal", "signal", signal)
	provider.Shutdown(context.Background())
	logger.Info("ending go routine")
}

// TODO-MEDIUM: add support for Prometheus metrics (using OpenTelemetry)
// we definitely want the memory usage and garbage collection metrics (see the Go Collector from https://github.com/prometheus/client_golang/blob/main/examples/gocollector/main.go)
// also https://povilasv.me/prometheus-go-metrics/

// TODO-LOW: if we want to associate a trace with logs: https://github.com/go-slog/otelslog
