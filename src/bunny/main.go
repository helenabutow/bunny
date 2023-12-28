package main

import (
	"bunny/config"
	"bunny/egress"
	"bunny/ingress"
	"bunny/logging"
	"bunny/otel"
	"bunny/signals"
	"log/slog"
	"os"
	"runtime/debug"
	"sync"
)

func main() {
	var logger *slog.Logger = logging.ConfigureLogger("main")
	logger.Info("begin")

	// TODO-LOW: write docs on how users should set the GOMEMLIMIT and GOGC env vars based on need (with reference to https://tip.golang.org/doc/gc-guide)
	// with the defaults below, this seems to keep go_memstats_alloc_bytes at roughly 2-4 megs when idle
	goMemLimitEnvVar := os.Getenv("GOMEMLIMIT")
	goGCEnvVar := os.Getenv("GOGC")
	if goMemLimitEnvVar == "" {
		debug.SetMemoryLimit(1024 * 1024 * 64) // 64 megs
	}
	if goGCEnvVar == "" {
		debug.SetGCPercent(10)
	}

	// wiring up channels
	config.AddChannelListener(&egress.ConfigUpdateChannel)
	config.AddChannelListener(&ingress.ConfigUpdateChannel)
	config.AddChannelListener(&signals.ConfigUpdateChannel)
	signals.AddChannelListener(&config.OSSignalsChannel)
	signals.AddChannelListener(&egress.OSSignalsChannel)
	signals.AddChannelListener(&ingress.OSSignalsChannel)
	signals.AddChannelListener(&otel.OSSignalsChannel)

	// do the rest of each package's init
	config.Init(logging.ConfigureLogger("config"))
	egress.Init(logging.ConfigureLogger("egress"))
	ingress.Init(logging.ConfigureLogger("ingress"))
	otel.Init(logging.ConfigureLogger("otel"))
	signals.Init(logging.ConfigureLogger("signals"))

	// start each go routinue for each package that has one
	var wg sync.WaitGroup
	go config.GoConfig(&wg)
	wg.Add(1)
	go egress.GoEgress(&wg)
	wg.Add(1)
	go ingress.GoIngress(&wg)
	wg.Add(1)
	go otel.GoOTel(&wg)
	wg.Add(1)
	go signals.GoSignals(&wg)
	wg.Add(1)
	wg.Wait()

	logger.Info("end")
}
