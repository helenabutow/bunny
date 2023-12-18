package ingress

import (
	"bunny/config"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
)

var logger *log.Logger = nil
var ingressConfig *config.IngressConfig = nil
var healthEndpointServer *http.Server = nil

func GoIngress(wg *sync.WaitGroup, configUpdateChannel chan config.BunnyConfig, osSignalsChannel chan os.Signal) {
	defer wg.Done()

	logger = log.Default()
	logger.Println("Ingress is go!")

	for {
		logger.Println("waiting for config or signal")
		select {
		case bunnyConfig, ok := <-configUpdateChannel:
			if !ok {
				continue
			}
			logger.Println("received config update")
			ingressConfig = &bunnyConfig.IngressConfig
			shutdownHealthEndpoint()
			startHealthEndpoint()
			logger.Println("config update processing complete")

		case signal, ok := <-osSignalsChannel:
			if !ok {
				logger.Println("could not process signal from signal channel")
			}
			logger.Printf("received signal %v. Ending go routine.", signal)
			shutdownHealthEndpoint()
			return
		}
	}

	// logger.Println("Ingress is done!")
}

func shutdownHealthEndpoint() {
	if healthEndpointServer != nil {
		logger.Println("shutting down health endpoint server")
		err := healthEndpointServer.Shutdown(context.Background())
		if err != nil {
			logger.Printf("errors while shutting down the health endpoint server: %v", err)
		}
		logger.Println("done shutting down health endpoint server")
	}
}

func startHealthEndpoint() {
	logger.Println("starting health endpoint server")
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
			logger.Fatalf("HTTP server ListenAndServe: %v", err)
		}
	}()
	logger.Println("done starting health endpoint server")
}

func healthEndpoint(w http.ResponseWriter, req *http.Request) {
	logger.Println("healthy")
	fmt.Fprintln(w, "healthy")
}

// TODO-LOW: add rate limiting - see https://gobyexample.com/rate-limiting
