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
	logger.Info("begin")

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
	signals.AddChannelListener(&telemetry.OSSignalsChannel)

	// do the rest of each package's init
	config.Init(logging.ConfigureLogger("config"))
	egress.Init(logging.ConfigureLogger("egress"))
	ingress.Init(logging.ConfigureLogger("ingress"))
	telemetry.Init(logging.ConfigureLogger("telemetry"))
	signals.Init(logging.ConfigureLogger("signals"))

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
