package otel

import (
	"bunny/config"
	"context"
	"log"
	"log/slog"
	"os"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	api "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/metric"
)

var logger *slog.Logger = nil
var ConfigUpdateChannel chan config.BunnyConfig = make(chan config.BunnyConfig, 1)
var OSSignalsChannel chan os.Signal = make(chan os.Signal, 1)
var oTelConfig *config.OTelConfig = nil

func Init(sharedLogger *slog.Logger) {
	logger = sharedLogger
	logger.Info("OTel initializing")

	exporter, err := prometheus.New()
	if err != nil {
		logger.Error("error while creating Prometheus exporter", "err", err)
	}
	provider := metric.NewMeterProvider(metric.WithReader(exporter))
	meter := provider.Meter(oTelConfig.MeterName)
	// TODO-HIGH: move this test data creation into a separate metrics package?
	opt := api.WithAttributes(
		attribute.Key("A").String("B"),
		attribute.Key("C").String("D"),
	)
	ctx := context.Background()
	counter, err := meter.Float64Counter("foo", api.WithDescription("a simple counter"))
	if err != nil {
		log.Fatal(err)
	}
	counter.Add(ctx, 5, opt)

	logger.Info("OTel is initialized")
}

func GoOTel(wg *sync.WaitGroup) {
	defer wg.Done()

	logger.Info("OTel is go!")

	for {
		logger.Info("waiting for config or signal")
		select {
		case bunnyConfig, ok := <-ConfigUpdateChannel:
			if !ok {
				continue
			}
			logger.Info("received config update")
			oTelConfig = &bunnyConfig.OTelConfig
			logger.Info("config update processing complete")

		case signal, ok := <-OSSignalsChannel:
			if !ok {
				logger.Error("could not process signal from signal channel")
			}
			logger.Info("received signal", "signal", signal)
			logger.Info("ending go routine")
			return
		}
	}
}

// TODO-MEDIUM: add support for Prometheus metrics (using OpenTelemetry)
// we might want to use this for deploying a Prometheus scraper into the cluster: https://github.com/tilt-dev/tilt-extensions/tree/master/helm_resource
// we definitely want the memory usage and garbage collection metrics (see the Go Collector from https://github.com/prometheus/client_golang/blob/main/examples/gocollector/main.go)
// also https://povilasv.me/prometheus-go-metrics/

// TODO-LOW: if we want to associate a trace with logs: https://github.com/go-slog/otelslog
