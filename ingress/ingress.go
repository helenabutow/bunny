package ingress

import (
	"bunny/config"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
)

var logger *slog.Logger = slog.Default()
var ConfigUpdateChannel chan config.BunnyConfig = make(chan config.BunnyConfig, 1)
var OSSignalsChannel chan os.Signal = make(chan os.Signal, 1)
var ingressConfig *config.IngressConfig = nil
var healthEndpointServer *http.Server = nil

func Init() {
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
			shutdownHealthEndpoint()
			startHealthEndpoint()
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
	if healthEndpointServer != nil {
		logger.Info("shutting down health endpoint server")
		err := healthEndpointServer.Shutdown(context.Background())
		if err != nil {
			logger.Error("errors while shutting down the health endpoint server", "err", err)
		}
		logger.Info("done shutting down health endpoint server")
	}
}

func startHealthEndpoint() {
	logger.Info("starting health endpoint server")
	mux := http.NewServeMux()
	mux.HandleFunc("/"+ingressConfig.Path, healthEndpoint)
	// TODO-LOW: tweak http timeouts to something helpful?
	healthEndpointServer = &http.Server{
		Addr:              ":" + fmt.Sprintf("%d", ingressConfig.Port),
		ReadTimeout:       0,
		ReadHeaderTimeout: 0,
		WriteTimeout:      0,
		IdleTimeout:       0,
		MaxHeaderBytes:    0,
		Handler:           mux,
	}

	go func() {
		err := healthEndpointServer.ListenAndServe()
		if err != http.ErrServerClosed {
			// Error starting or closing listener:
			logger.Error("Error starting or closing listener", "err", err)
		}
	}()
	logger.Info("done starting health endpoint server")
}

// TODO-MEDIUM: consider allowing PromQL to be used to determine endpoint result
// PromQL intro and reference: https://prometheus.io/docs/prometheus/latest/querying/basics/
// We may be able to implement this against the Prometheus metrics endpoint that OpenTelemetry provides
// If that doesn't work, we could also consider running a Prometheus server as a sidecar
// (though tuning its storage and memory usage could be a pain: https://prometheus.io/docs/prometheus/1.8/storage/)
// And if that doesn't work, we could also support using PromQL against an external server
// (though the round trip time for getting the newest metrics might make that less useful)
func healthEndpoint(w http.ResponseWriter, req *http.Request) {
	logger.Info("healthy")
	fmt.Fprintln(w, "healthy")
}

// TODO-LOW: add rate limiting - see https://gobyexample.com/rate-limiting
