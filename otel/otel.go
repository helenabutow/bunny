package otel

import (
	"bunny/config"
	"log"
	"os"
	"sync"
)

var logger *log.Logger = log.Default()
var ConfigUpdateChannel chan config.BunnyConfig = make(chan config.BunnyConfig, 1)
var OSSignalsChannel chan os.Signal = make(chan os.Signal, 1)
var oTelConfig *config.OTelConfig = nil

func Init() {
	logger.Println("OTel initializing")
	logger.Println("OTel is initialized")
}

func GoOTel(wg *sync.WaitGroup) {
	defer wg.Done()

	logger.Println("OTel is go!")

	for {
		logger.Println("waiting for config or signal")
		select {
		case bunnyConfig, ok := <-ConfigUpdateChannel:
			if !ok {
				continue
			}
			logger.Println("received config update")
			oTelConfig = &bunnyConfig.OTelConfig
			logger.Println("config update processing complete")

		case signal, ok := <-OSSignalsChannel:
			if !ok {
				logger.Println("could not process signal from signal channel")
			}
			logger.Printf("received signal %v", signal)
			logger.Printf("ending go routine")
			return
		}
	}
}

// TODO-MEDIUM: add support for Prometheus metrics (using OpenTelemetry)
// we might want to use this for deploying a Prometheus scraper into the cluster: https://github.com/tilt-dev/tilt-extensions/tree/master/helm_resource
// we definitely want the memory usage and garbage collection metrics (see the Go Collector from https://github.com/prometheus/client_golang/blob/main/examples/gocollector/main.go)
// also https://povilasv.me/prometheus-go-metrics/
