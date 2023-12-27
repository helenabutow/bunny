package main

import (
	"bunny/config"
	"bunny/egress"
	"bunny/ingress"
	"bunny/otel"
	"bunny/signals"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/golang-cz/devslog"
)

func main() {
	var logger *slog.Logger = configureLogger("main")
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
	config.Init(configureLogger("config"))
	egress.Init(configureLogger("egress"))
	ingress.Init(configureLogger("ingress"))
	otel.Init(configureLogger("otel"))
	signals.Init(configureLogger("signals"))

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

func configureLogger(packageName string) *slog.Logger {
	// TODO-LOW: support setting the log level via the config file as well
	// (so that the initial log level is set via an env var and then is changeable via the config file)
	// we may want to support having different log levels for different packages
	var logLevel = new(slog.LevelVar)
	logLevelEnvVar := os.Getenv(strings.ToUpper(packageName) + "_LOG_LEVEL")
	if logLevelEnvVar != "" {
		switch logLevelEnvVar {
		case "INFO", "info":
			logLevel.Set(slog.LevelInfo)
		case "DEBUG", "debug":
			logLevel.Set(slog.LevelDebug)
		case "WARN", "warn":
			logLevel.Set(slog.LevelWarn)
		case "ERROR", "error":
			logLevel.Set(slog.LevelError)
		default:
			logLevel.Set(slog.LevelInfo)
		}
	}
	var handlerOptions = slog.HandlerOptions{
		AddSource: true,
		Level:     logLevel,
	}
	var logger *slog.Logger = nil
	logHandlerEnvVar := os.Getenv("LOG_HANDLER")
	switch logHandlerEnvVar {
	case "TEXT", "text", "CONSOLE", "console":
		devSlogOpts := &devslog.Options{
			HandlerOptions:    &handlerOptions,
			MaxSlicePrintSize: 100,
			SortKeys:          true,
		}
		logger = slog.New(devslog.NewHandler(os.Stdout, devSlogOpts))

	default:
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &handlerOptions))
	}
	slog.SetDefault(logger)
	return logger
}