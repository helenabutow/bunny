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
var provider *metric.MeterProvider = nil
var meter *api.Meter = nil

func Init(sharedLogger *slog.Logger) {
	logger = sharedLogger
	logger.Info("OTel initializing")

	// setup Prometheus
	// the HTTP endpoint is in the ingress package
	exporter, err := prometheus.New()
	if err != nil {
		logger.Error("error while creating Prometheus exporter", "err", err)
	}
	provider = metric.NewMeterProvider(metric.WithReader(exporter))
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
			// TODO-HIGH: it looks like we can't just create a new Meter if we want to rename the meter
			// to repro, rename `otel.meterName` in `deploy/local/bunny.yaml`, wait a minute and then rename it back
			// you should see the the `foo_total` metric in Mimir double with 2 `otel_scope_name` label values
			// ick
			newMeter := provider.Meter(oTelConfig.MeterName)
			meter = &(newMeter)

			// TODO-HIGH: move this test data creation into a separate metrics package?
			opt := api.WithAttributes(
				attribute.Key("A").String("B"),
				attribute.Key("C").String("D"),
			)
			ctx := context.Background()
			counter, err := (*meter).Float64Counter("foo", api.WithDescription("a simple counter"))
			if err != nil {
				log.Fatal(err)
			}
			counter.Add(ctx, 5, opt)

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

// TODO-LOW: each metric that ingress generates should toggle-able
// if someone doesn't need a metric, we should waste cpu generating values for it
// and if they're opt-in, scrape configs get simpler and don't have to change as metrics are added/removed
// (which would be a pain if someone was using annotation based scrape configs on their Pods)
