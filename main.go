package main

import (
	"bunny/config"
	"bunny/ingress"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

func main() {
	var logger = log.Default()
	logger.SetFlags(log.Ldate | log.Ltime | log.Llongfile | log.LUTC)
	logger.Println("begin")

	// TODO-LOW: set a memory limit (using runtime/debug.SetMemoryLimit, if not already set via the GOMEMLIMIT env var)
	// TODO-LOW: set garbage collection (using runtime/debug.SetGCPercent, if not already set via the GOGC env var)

	// TODO-LOW: create channels
	var configUpdateChannel chan config.BunnyConfig = make(chan config.BunnyConfig, 1)
	var osSignalsChannel chan os.Signal = make(chan os.Signal, 1)

	// TODO-HIGH: we need to refactor out the channel listener registration out of each top level go routine
	// register channel listeners
	config.AddChannelListener()

	// register the OS signals that we want to receive
	// TODO-HIGH: this doesn't work the way we expected. We need to switch to creating a top-level go routine (similar to GoConfig) which sends out a message on a different channel
	// TODO-HIGH: do we wait until the app process exits? (since we should be in the same PID namespace, does that seem reasonable?)
	signal.Notify(osSignalsChannel, syscall.SIGINT, syscall.SIGTERM)

	// TODO-LOW: create more go routines
	var wg sync.WaitGroup
	// TODO-HIGH: need to make sure that we notify every listener
	// increment this as we add more top level go routines
	go config.GoConfig(&wg, configUpdateChannel, osSignalsChannel, 1)
	wg.Add(1)
	go ingress.GoIngress(&wg, configUpdateChannel, osSignalsChannel)
	wg.Add(1)
	wg.Wait()

	logger.Println("end")
}

// TODO-MEDIUM: add support for Prometheus metrics
// we might want to use this for deploying a Prometheus scraper into the cluster: https://github.com/tilt-dev/tilt-extensions/tree/master/helm_resource
// we definitely want the memory usage and garbage collection metrics (see the Go Collector from https://github.com/prometheus/client_golang/blob/main/examples/gocollector/main.go)
// also https://povilasv.me/prometheus-go-metrics/
