package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"

	kitlog "github.com/go-kit/log"
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

type slogAdapterLogger struct {
	slog.Logger
}

func NewSlogAdapterLogger() kitlog.Logger {
	return &slogAdapterLogger{
		*slog.Default(),
	}
}

func (l *slogAdapterLogger) Log(keyvals ...interface{}) error {
	if !l.Logger.Enabled(context.Background(), slog.LevelInfo) {
		return nil
	}

	// convert keyvals into a map
	n := (len(keyvals) + 1) / 2 // +1 to handle case when len is odd
	m := make(map[string]interface{}, n)
	for i := 0; i < len(keyvals); i += 2 {
		k := keyvals[i]
		var v interface{} = "MISSING"
		if i+1 < len(keyvals) {
			v = keyvals[i+1]
		}
		m[fmt.Sprint(k)] = v
	}

	// deal with the log level and msg keys (if they're present)
	var level string = "info"
	var msg string = ""
	for k, v := range m {
		switch k {
		case "level":
			level = fmt.Sprint(v)
			delete(m, k)
		case "msg":
			msg = fmt.Sprint(v)
			delete(m, k)
		case "caller": // slog provides this already with a complete path, so we can just drop it
			delete(m, k)
		}
	}
	flat := []any{}
	for k, v := range m {
		flat = append(flat, k)
		flat = append(flat, fmt.Sprint(v))
	}
	var slogLevel = slog.LevelInfo
	switch level {
	case "INFO", "info":
		slogLevel = slog.LevelInfo
	case "DEBUG", "debug":
		slogLevel = slog.LevelDebug
	case "WARN", "warn":
		slogLevel = slog.LevelWarn
	case "ERROR", "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	var pcs [1]uintptr
	runtime.Callers(3, pcs[:]) // skip [Callers, Infof]
	r := slog.NewRecord(time.Now(), slogLevel, msg, pcs[0])
	r.Add(flat...)
	var err error = l.Logger.Handler().Handle(context.Background(), r)
	return err
}
