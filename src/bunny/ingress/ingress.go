package ingress

import (
	"bunny/config"
	"bunny/logging"
	"bunny/telemetry"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var logger *slog.Logger = nil
var ConfigUpdateChannel chan config.BunnyConfig = make(chan config.BunnyConfig, 1)
var OSSignalsChannel chan os.Signal = make(chan os.Signal, 1)
var ConfigStageChannel chan config.ConfigStage = make(chan config.ConfigStage, 1)
var ingressConfig *config.IngressConfig = nil
var meter *metric.Meter = nil
var httpServer *http.Server = nil
var healthEndpoints [](*HealthEndpoint) = [](*HealthEndpoint){}

func GoIngress(wg *sync.WaitGroup) {
	defer wg.Done()

	logger = logging.ConfigureLogger("ingress")
	logger.Info("Ingress is go!")

	for {
		logger.Debug("waiting for config or signal")
		select {
		case bunnyConfig, ok := <-ConfigUpdateChannel:
			if !ok {
				logger.Error("could not process config from config update channel")
				continue
			}
			logger.Info("received config update")
			ingressConfig = &bunnyConfig.Ingress

			// wait until telemetry finishes processing its config
			configStage, ok := <-ConfigStageChannel
			if !ok {
				logger.Error("ConfigStageChannel is not ok. Returning")
				return
			}
			if configStage != config.ConfigStageTelemetryCompleted {
				logger.Error("unknown config stage. Returning")
				return
			}

			newMeter := otel.GetMeterProvider().Meter("bunny/ingress")
			meter = &newMeter

			// process config for health endpoints
			healthEndpoints = [](*HealthEndpoint){}
			for _, healthConfig := range ingressConfig.HTTPServerConfig.Health {
				healthEndpoint, err := newHealthEndpoint(&healthConfig)
				if err != nil {
					logger.Error("error while processing config for health endpoint", "healthEndpoint", healthEndpoint)
					continue
				}
				healthEndpoints = append(healthEndpoints, healthEndpoint)
			}

			shutdownHTTPServer()
			startHTTPServer()

			logger.Info("config update processing complete")

		case signal, ok := <-OSSignalsChannel:
			if !ok {
				logger.Error("could not process signal from signal channel")
			}
			logger.Info("received signal. Ending go routine.", "signal", signal)
			shutdownHTTPServer()
			logger.Info("completed shutdowns. Returning from go routine")
			return
		}
	}
}

func shutdownHTTPServer() {
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

	// Health endpoints handlers
	for _, healthConfig := range ingressConfig.HTTPServerConfig.Health {
		// rather than having to dynamically create a func per endpoint, we handle each endpoint
		// through the same func and figure out which query to make based on the request path
		mux.HandleFunc(ensureLeadingSlash(healthConfig.Path), healthEndpointHandler)
	}

	// OpenTelemetry metrics handler
	mux.Handle(ensureLeadingSlash(ingressConfig.HTTPServerConfig.OpenTelemetryMetricsPath), promhttp.Handler())

	// Prometheus metrics handler
	// TODO-LOW: we're using most of the default options for the handler here. We might need to tweak that.
	handlerOpts := promhttp.HandlerOpts{Registry: telemetry.PromRegistry}
	mux.Handle(ensureLeadingSlash(ingressConfig.HTTPServerConfig.PrometheusMetricsPath),
		promhttp.HandlerFor(telemetry.PromRegistry, handlerOpts))

	httpServer = &http.Server{
		Addr:              ":" + fmt.Sprintf("%d", ingressConfig.HTTPServerConfig.Port),
		ReadTimeout:       time.Duration(ingressConfig.HTTPServerConfig.ReadTimeoutMilliseconds) * time.Millisecond,
		ReadHeaderTimeout: time.Duration(ingressConfig.HTTPServerConfig.ReadHeaderTimeoutMilliseconds) * time.Millisecond,
		WriteTimeout:      time.Duration(ingressConfig.HTTPServerConfig.WriteTimeoutMilliseconds) * time.Millisecond,
		IdleTimeout:       time.Duration(ingressConfig.HTTPServerConfig.IdleTimeoutMilliseconds) * time.Millisecond,
		MaxHeaderBytes:    ingressConfig.HTTPServerConfig.MaxHeaderBytes,
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

func ensureLeadingSlash(path string) string {
	if strings.Index(path, "/") != 0 {
		return "/" + path
	}
	return path
}

func healthEndpointHandler(w http.ResponseWriter, req *http.Request) {
	// find the HealthEndpoint for the path in the request and execute the query for it
	for _, healthEndpoint := range healthEndpoints {
		if healthEndpoint.Path == req.URL.Path {
			logger.Debug("execing query", "healthEndpoint", healthEndpoint)
			queryResult, err := (*healthEndpoint.Query).exec(healthEndpoint.AttemptsMetric, healthEndpoint.ResponseTimeMetric)
			if err != nil {
				logger.Error("error while executing query for health endpoint",
					"healthEndpoint", healthEndpoint,
					"err", err,
				)
				queryResult = false
			}
			if queryResult {
				logger.Debug("healthy")
				w.WriteHeader(http.StatusOK)
				fmt.Fprintln(w, "healthy")
			} else {
				logger.Debug("unhealthy")
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprintln(w, "unhealthy")
			}
			return
		}
	}
	logger.Error("no endpoint found for path", "req.URL.Path", req.URL.Path)
}

// TODO-LOW: add rate limiting - see https://gobyexample.com/rate-limiting
// instead, we might have to do this via the `healthEndpoint` func by keeping a request rate metric
// and rejecting requests that exceed the rate
// see: https://www.alexedwards.net/blog/how-to-rate-limit-http-requests
