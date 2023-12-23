package main

import (
	"bunny/config"
	"bunny/ingress"
	"bunny/otel"
	"bunny/signals"
	"log"
	"sync"
)

var logger = log.Default()

func main() {
	logger.SetFlags(log.Ldate | log.Ltime | log.Llongfile | log.LUTC)
	logger.Println("begin")

	// TODO-LOW: set a memory limit (using runtime/debug.SetMemoryLimit, if not already set via the GOMEMLIMIT env var)
	// TODO-LOW: set garbage collection (using runtime/debug.SetGCPercent, if not already set via the GOGC env var)

	// wiring up channels
	config.AddChannelListener(&ingress.ConfigUpdateChannel)
	config.AddChannelListener(&otel.ConfigUpdateChannel)
	config.AddChannelListener(&signals.ConfigUpdateChannel)
	signals.AddChannelListener(&config.OSSignalsChannel)
	signals.AddChannelListener(&ingress.OSSignalsChannel)
	signals.AddChannelListener(&otel.OSSignalsChannel)

	// do the rest of each package's init
	config.Init()
	ingress.Init()
	signals.Init()

	// start each go routinue for each package that has one
	var wg sync.WaitGroup
	go config.GoConfig(&wg)
	wg.Add(1)
	go ingress.GoIngress(&wg)
	wg.Add(1)
	go otel.GoOTel(&wg)
	wg.Add(1)
	go signals.GoSignals(&wg)
	wg.Add(1)
	wg.Wait()

	logger.Println("end")
}
