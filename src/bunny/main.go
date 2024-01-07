package main

import (
	"bunny/config"
	"bunny/egress"
	"bunny/ingress"
	"bunny/logging"
	"bunny/signals"
	"bunny/telemetry"
	"log/slog"
	"os"
	"runtime/debug"
	"sync"
)

func main() {
	var logger *slog.Logger = logging.ConfigureLogger("main")
	// this implies that dependencies which still use log instead of slog, use the logger for main
	slog.SetDefault(logger)

	logger.Info("begin")

	// we have to do this super early in case GOMEMLIMIT is set really low
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
	config.AddChannelListener(&telemetry.ConfigUpdateChannel)
	telemetry.AddChannelListener(&egress.ConfigStageChannel)
	telemetry.AddChannelListener(&ingress.ConfigStageChannel)
	signals.AddChannelListener(&config.OSSignalsChannel)
	signals.AddChannelListener(&egress.OSSignalsChannel)
	signals.AddChannelListener(&ingress.OSSignalsChannel)
	signals.AddChannelListener(&telemetry.OSSignalsChannel)

	// start each go routinue for each package that has one
	var wg sync.WaitGroup
	go config.GoConfig(&wg)
	wg.Add(1)
	go egress.GoEgress(&wg)
	wg.Add(1)
	go ingress.GoIngress(&wg)
	wg.Add(1)
	go telemetry.GoTelemetry(&wg)
	wg.Add(1)
	go signals.GoSignals(&wg)
	wg.Add(1)
	wg.Wait()

	logger.Info("end")
}
