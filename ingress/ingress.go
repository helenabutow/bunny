package ingress

import (
	"bunny/config"
	"bunny/otel"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	api "go.opentelemetry.io/otel/metric"
)

var logger *slog.Logger = nil
var ConfigUpdateChannel chan config.BunnyConfig = make(chan config.BunnyConfig, 1)
var OSSignalsChannel chan os.Signal = make(chan os.Signal, 1)
var ingressConfig *config.IngressConfig = nil
var meter *api.Meter = nil
var extraAttributes *api.MeasurementOption = nil
var healthAttempts int64 = 0
var healthAttemptsCounter *api.Int64Counter = nil
var httpServer *http.Server = nil

func Init(sharedLogger *slog.Logger) {
	logger = sharedLogger
	logger.Info("Ingress initializing")
	logger.Info("Ingress is initialized")
}

func GoIngress(wg *sync.WaitGroup) {
	defer wg.Done()

	logger.Info("Ingress is go!")

	for {
		logger.Info("waiting for config or signal")
		select {
		case bunnyConfig, ok := <-ConfigUpdateChannel:
			if !ok {
				continue
			}
			logger.Info("received config update")
			ingressConfig = &bunnyConfig.IngressConfig

			meter = otel.Meter

			// TODO-LOW: replace these attributes with ones read from config
			// extra key/value pairs to include with each Prometheus metric
			newExtraAttributes := api.WithAttributes(
				attribute.Key("A").String("B"),
				attribute.Key("C").String("D"),
			)
			extraAttributes = &newExtraAttributes

			newHealthAttemptsCounter, err := (*meter).Int64Counter("ingress_health_attempts", api.WithDescription("the number of connections attempted to the health endpoint"))
			if err != nil {
				logger.Error("could not create healthAttemptsCounter", "err", err)
			}
			healthAttemptsCounter = &newHealthAttemptsCounter

			shutdownHealthEndpoint()
			startHTTPServer()

			logger.Info("config update processing complete")

		case signal, ok := <-OSSignalsChannel:
			if !ok {
				logger.Error("could not process signal from signal channel")
			}
			logger.Info("received signal. Ending go routine.", "signal", signal)
			shutdownHealthEndpoint()
			return
		}
	}
}

func shutdownHealthEndpoint() {
	if httpServer != nil {
		logger.Info("shutting down health endpoint server")
		err := httpServer.Shutdown(context.Background())
		if err != nil {
			logger.Error("errors while shutting down the health endpoint server", "err", err)
		}
		logger.Info("done shutting down health endpoint server")
	}
}

func startHTTPServer() {
	logger.Info("starting HTTP server")
	mux := http.NewServeMux()
	// TODO-MEDIUM: make *all* the endpoint paths configurable
	// we have to do this because if we're running bunny as a sidecar, there will be conflicts with the "/metrics" endpoint for the app
	mux.HandleFunc("/"+ingressConfig.Path, healthEndpoint)
	mux.Handle("/metrics", promhttp.Handler())
	// TODO-LOW: tweak http timeouts to something helpful?
	httpServer = &http.Server{
		Addr:              ":" + fmt.Sprintf("%d", ingressConfig.Port),
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
			// Error starting or closing listener:
			logger.Error("Error starting or closing listener", "err", err)
		}
	}()
	logger.Info("done starting HTTP server")
}

// TODO-MEDIUM: consider allowing PromQL to be used to determine endpoint result
// PromQL intro and reference: https://prometheus.io/docs/prometheus/latest/querying/basics/
// We may be able to implement this against the Prometheus metrics endpoint that OpenTelemetry provides
// If that doesn't work, we could also consider running a Prometheus server as a sidecar
// (though tuning its storage and memory usage could be a pain: https://prometheus.io/docs/prometheus/1.8/storage/)
// And if that doesn't work, we could also support using PromQL against an external server
// (though the round trip time for getting the newest metrics might make that less useful)
// this might be a starting point: https://github.com/prometheus/client_golang/blob/main/api/prometheus/v1/example_test.go#L54
// (we should check if OpenTelemetry has a way to do this)
func healthEndpoint(w http.ResponseWriter, req *http.Request) {
	logger.Debug("healthy")

	healthAttempts++
	logger.Debug("healthAttempts has increased", "healthAttempts", healthAttempts)
	(*healthAttemptsCounter).Add(context.Background(), 1, *extraAttributes)

	fmt.Fprintln(w, "healthy")
}

// TODO-LOW: add rate limiting - see https://gobyexample.com/rate-limiting

// TODO-HIGH: go golang tickers skew? Run an overnight test to figure it out
// if we're going to use tickers in the selects for ingress and egress to
