package main

import (
	"bunny/logging"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var logger *slog.Logger = nil

var httpServer *http.Server = nil

func main() {
	logger = logging.ConfigureLogger("main")
	logger.Info("begin")

	startHTTPEndpoint()

	// wait for OS signal
	var osSignalsChannel chan os.Signal = make(chan os.Signal, 1)
	signal.Notify(osSignalsChannel, syscall.SIGINT, syscall.SIGTERM)
	signal, ok := <-osSignalsChannel
	if !ok {
		logger.Error("could not process signal from signal channel")
	}
	logger.Info("received signal", "signal", signal)

	logger.Info("end")
}

func startHTTPEndpoint() {
	logger.Info("starting HTTP server")
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthEndpoint)

	httpServer = &http.Server{
		Addr:              ":" + fmt.Sprintf("%d", 2624),
		ReadTimeout:       0,
		ReadHeaderTimeout: 0,
		WriteTimeout:      0,
		IdleTimeout:       0,
		MaxHeaderBytes:    0,
		Handler:           mux,
	}

	go func() {
		err := httpServer.ListenAndServe()
		if err != http.ErrServerClosed {
			logger.Error("Error starting or closing listener", "err", err)
		}
	}()
	logger.Info("done starting HTTP server")
}

func healthEndpoint(response http.ResponseWriter, request *http.Request) {
	logger.Debug("headers", "request.Header", request.Header)

	// health is based on a a sine wave
	const period int64 = 100
	const maxDelay float64 = 1.0
	var percent = float64(time.Now().Unix()%period) / float64(period)
	var x float64 = 2.0 * (float64(math.Pi)) * percent
	var y = (math.Sin(x) + 1.0) / 2.0
	var delay = time.Duration(y * maxDelay * float64(time.Second))
	logger.Debug("health calc",
		"percent", percent,
		"x", x,
		"y", y,
		"delay", delay,
	)
	time.Sleep(delay)
	if y > 0.0 {
		logger.Debug("healthy")
		response.WriteHeader(http.StatusOK)
		fmt.Fprintln(response, "healthy")
	} else {
		logger.Debug("unhealthy")
		response.WriteHeader(http.StatusRequestTimeout)
		fmt.Fprintln(response, "unhealthy")
	}
}
