package logging

import (
	"log/slog"
	"os"
	"strings"

	"github.com/golang-cz/devslog"
)

func ConfigureLogger(packageName string) *slog.Logger {
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
