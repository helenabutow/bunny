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
var healthAttempts int64 = 0
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

func healthEndpoint(w http.ResponseWriter, req *http.Request) {
	healthAttempts++
	logger.Debug("healthAttempts has increased", "healthAttempts", healthAttempts)

	// health is based on a a sine wave
	const period int64 = 10
	const maxDelay float64 = 3.0
	var top = float64(time.Now().Unix() % period)
	var bottom = (float64(math.Pi)) / 2.0
	var x float64 = top / bottom
	var y = math.Sin(x)
	var delay = time.Duration(math.Abs(y) * maxDelay * float64(time.Second))
	logger.Debug("health calc", "top", top)
	logger.Debug("health calc", "bottom", bottom)
	logger.Debug("health calc", "x", x)
	logger.Debug("health calc", "y", y)
	logger.Debug("health calc", "delay", delay)
	time.Sleep(delay)
	if y > 0.0 {
		logger.Debug("healthy")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "healthy")
	} else {
		logger.Debug("unhealthy")
		w.WriteHeader(http.StatusRequestTimeout)
		fmt.Fprintln(w, "unhealthy")
	}
}
