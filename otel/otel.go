package otel

import (
	"bunny/config"
	"log/slog"
	"os"
	"sync"
)

var logger *slog.Logger = slog.Default()
var ConfigUpdateChannel chan config.BunnyConfig = make(chan config.BunnyConfig, 1)
var OSSignalsChannel chan os.Signal = make(chan os.Signal, 1)
var oTelConfig *config.OTelConfig = nil

func Init() {
	logger.Info("OTel initializing")
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
