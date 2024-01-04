package ingress

import (
	"bunny/config"
	"bunny/telemetry"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
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
			newMeter := otel.GetMeterProvider().Meter("bunny/ingress")
			meter = &newMeter

			// extra key/value pairs to include with each OpenTelemetry metric
			attributesCopy := make([]attribute.KeyValue, len(ingressConfig.IngressPrometheusConfig.ExtraIngressPrometheusLabels))
			for i, promLabelConfig := range ingressConfig.IngressPrometheusConfig.ExtraIngressPrometheusLabels {
				attributesCopy[i] = attribute.Key(promLabelConfig.Name).String(promLabelConfig.Value)
			}
			newExtraAttributes := metric.WithAttributeSet(attribute.NewSet(attributesCopy...))
			extraAttributes = &newExtraAttributes

			// OpenTelemetry metrics
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

	// Health endpoints handlers
	for _, healthConfig := range ingressConfig.HTTPServerConfig.HealthConfig {
		// rather than having to dynamically create a func per endpoint, we handle each endpoint
		// through the same func and figure out which query to make based on the request path
		mux.HandleFunc(ensureLeadingSlash(healthConfig.Path), healthEndpoint)
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

func healthEndpoint(w http.ResponseWriter, req *http.Request) {
	logger.Debug("healthy")

	// TODO-HIGH: we need separate health attempt counters per health endpoint
	healthAttempts++
	logger.Debug("healthAttempts has increased", "healthAttempts", healthAttempts)
	(*healthAttemptsCounter).Add(context.Background(), 1, *extraAttributes)

	// TODO-LOW: we could optimize this by moving most of this code into the config update block of GoIngress
	// the exception being time.Time values (as those are relative to time.Now()) but time.Duration would be fine
	// It's also gross code that I'm writing while tired of dealing with YAML. I'm sorry.
	for _, healthConfig := range ingressConfig.HTTPServerConfig.HealthConfig {
		var instantQueryResult bool = true
		var rangeQueryResult bool = true
		if healthConfig.Path == req.URL.Path {
			if healthConfig.InstantQueryConfig != nil {
				duration, err := time.ParseDuration(healthConfig.InstantQueryConfig.QueryTimeout)
				if err != nil {
					logger.Error("error while parsing duration for queryTimeout",
						"healthConfig.InstantQueryConfig", healthConfig.InstantQueryConfig)
					return
				}
				query := healthConfig.InstantQueryConfig.Query
				instantTimeDuration, err := time.ParseDuration(healthConfig.InstantQueryConfig.QueryRelativeInstantTime)
				if err != nil {
					logger.Error("error while parsing duration for queryRelativeInstantTime",
						"healthConfig.InstantQueryConfig", healthConfig.InstantQueryConfig)
					return
				}
				instantTime := time.Now().Add(instantTimeDuration)
				instantQueryResult, _ = telemetry.InstantQuery(duration, query, instantTime)
			}
			if healthConfig.InstantQueryConfig != nil {
				duration, err := time.ParseDuration(healthConfig.RangeQueryConfig.QueryTimeout)
				if err != nil {
					logger.Error("error while parsing duration for queryTimeout",
						"healthConfig.RangeQueryConfig", healthConfig.RangeQueryConfig)
					return
				}
				query := healthConfig.RangeQueryConfig.Query
				startTimeDuration, err := time.ParseDuration(healthConfig.RangeQueryConfig.QueryRelativeStartTime)
				if err != nil {
					logger.Error("error while parsing duration for queryRelativeStartTime",
						"healthConfig.RangeQueryConfig", healthConfig.RangeQueryConfig)
					return
				}
				startTime := time.Now().Add(startTimeDuration)
				endTimeDuration, err := time.ParseDuration(healthConfig.RangeQueryConfig.QueryRelativeEndTime)
				if err != nil {
					logger.Error("error while parsing duration for queryRelativeEndTime",
						"healthConfig.RangeQueryConfig", healthConfig.RangeQueryConfig)
					return
				}
				endTime := time.Now().Add(endTimeDuration)
				interval, err := time.ParseDuration(healthConfig.RangeQueryConfig.QueryRelativeEndTime)
				if err != nil {
					logger.Error("error while parsing duration for queryRelativeEndTime",
						"healthConfig.RangeQueryConfig", healthConfig.RangeQueryConfig)
					return
				}
				rangeQueryResult, _ = telemetry.RangeQuery(duration, query, startTime, endTime, interval)
			}
			if instantQueryResult && rangeQueryResult {
				w.WriteHeader(http.StatusOK)
				fmt.Fprintln(w, "healthy")
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprintln(w, "unhealthy")
			}
			return
		}
	}
}

// TODO-LOW: add rate limiting - see https://gobyexample.com/rate-limiting
// instead, we might have to do this via the `healthEndpoint` func by keeping a request rate metric
// and rejecting requests that exceed the rate
// see: https://www.alexedwards.net/blog/how-to-rate-limit-http-requests
