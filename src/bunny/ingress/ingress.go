package ingress

import (
	"bunny/config"
	"bunny/otel"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var logger *slog.Logger = nil
var ConfigUpdateChannel chan config.BunnyConfig = make(chan config.BunnyConfig, 1)
var OSSignalsChannel chan os.Signal = make(chan os.Signal, 1)
var ingressConfig *config.IngressConfig = nil
var meter *metric.Meter = nil
var extraAttributes *metric.MeasurementOption = nil
var healthAttempts int64 = 0
var healthAttemptsCounter *metric.Int64Counter = nil
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
		logger.Debug("waiting for config or signal")
		select {
		case bunnyConfig, ok := <-ConfigUpdateChannel:
			if !ok {
				continue
			}
			logger.Info("received config update")
			ingressConfig = &bunnyConfig.IngressConfig

			meter = otel.Meter

			// extra key/value pairs to include with each Prometheus metric
			attributesCopy := make([]attribute.KeyValue, len(ingressConfig.IngressPrometheusConfig.ExtraIngressPrometheusLabels))
			for i, promLabelConfig := range ingressConfig.IngressPrometheusConfig.ExtraIngressPrometheusLabels {
				attributesCopy[i] = attribute.Key(promLabelConfig.Name).String(promLabelConfig.Value)
			}
			newExtraAttributes := metric.WithAttributeSet(attribute.NewSet(attributesCopy...))
			extraAttributes = &newExtraAttributes

			// Prometheus metrics
			var metricName string = "ingress_health_attempts"
			if slices.Contains(ingressConfig.IngressPrometheusConfig.MetricsEnabled, metricName) {
				newHealthAttemptsCounter, err := (*meter).Int64Counter(metricName)
				if err != nil {
					logger.Error("could not create healthAttemptsCounter", "err", err)
				}
				healthAttemptsCounter = &newHealthAttemptsCounter
			} else {
				healthAttemptsCounter = nil
			}

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
	mux.HandleFunc(ingressConfig.HTTPServerConfig.HealthPath, healthEndpoint)
	mux.Handle(ingressConfig.HTTPServerConfig.MetricsPath, promhttp.Handler())

	httpServer = &http.Server{
		Addr:              ":" + fmt.Sprintf("%d", ingressConfig.HTTPServerConfig.Port),
		ReadTimeout:       time.Duration(ingressConfig.HTTPServerConfig.ReadTimeout),
		ReadHeaderTimeout: time.Duration(ingressConfig.HTTPServerConfig.ReadHeaderTimeout),
		WriteTimeout:      time.Duration(ingressConfig.HTTPServerConfig.WriteTimeout),
		IdleTimeout:       time.Duration(ingressConfig.HTTPServerConfig.IdleTimeout),
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
