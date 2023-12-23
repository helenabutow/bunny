package main

import (
	"bunny/config"
	"bunny/ingress"
	"bunny/otel"
	"bunny/signals"
	"log/slog"
	"os"
	"sync"
)

func main() {
	var logger *slog.Logger = configureLogger()
	logger.Info("begin")

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
	config.Init(logger)
	ingress.Init(logger)
	otel.Init(logger)
	signals.Init(logger)

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

	logger.Info("end")
}

func configureLogger() *slog.Logger {
	// TODO-LOW: support setting the log level via the config file as well
	// (so that the initial log level is set via an env var and then is changeable via the config file)
	// TODO-LOW: support changing the timezone to UTC
	// this may be possible by just setting the TZ env var to "UTC"
	// or with https://github.com/samber/slog-formatter#TimeFormatter
	var logLevel = new(slog.LevelVar)
	logLevelEnvVar := os.Getenv("LOG_LEVEL")
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
		logger = slog.New(slog.NewTextHandler(os.Stdout, &handlerOptions))
	default:
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &handlerOptions))
	}
	slog.SetDefault(logger)
	return logger
}
