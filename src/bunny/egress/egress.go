package egress

import (
	"bunny/config"
	"log/slog"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

var logger *slog.Logger = nil
var ConfigUpdateChannel chan config.BunnyConfig = make(chan config.BunnyConfig, 1)
var OSSignalsChannel chan os.Signal = make(chan os.Signal, 1)
var ticker *time.Ticker = nil
var initialDelayTime time.Time = time.Now()
var egressConfig *config.EgressConfig = nil
var probes []Probe = []Probe{}
var meter *metric.Meter = nil

func Init(sharedLogger *slog.Logger) {
	logger = sharedLogger
	logger.Info("Egress initializing")

	// yes, this looks silly
	// the goal here is to prevent the ticker from firing until config has been loaded
	ticker = time.NewTicker(1 * time.Hour)
	ticker.Stop()

	logger.Info("Egress is initialized")
}

func GoEgress(wg *sync.WaitGroup) {
	defer wg.Done()

	logger.Info("Egress is go!")

	for {
		logger.Debug("waiting for config or signal")
		select {
		case tickTime := <-ticker.C:
			if initialDelayTime.Before(time.Now()) {
				performProbes(&tickTime)
			}

		case bunnyConfig, ok := <-ConfigUpdateChannel:
			if !ok {
				continue
			}
			updateConfig(&bunnyConfig)

		case signal, ok := <-OSSignalsChannel:
			if !ok {
				logger.Error("could not process signal from signal channel")
			}
			logger.Info("received signal. Ending go routine.", "signal", signal)
			return
		}
	}
}

func updateConfig(bunnyConfig *config.BunnyConfig) {
	logger.Info("received config update")
	egressConfig = &bunnyConfig.EgressConfig
	newMeter := otel.GetMeterProvider().Meter("bunny/egress")
	meter = &newMeter

	// process probe configs
	probes = []Probe{}
	for _, egressProbeConfig := range egressConfig.Probes {
		var newProbe *Probe = newProbe(&egressProbeConfig)
		probes = append(probes, *newProbe)
	}

	now := time.Now()
	initialDelayTime = now.Add(time.Duration(egressConfig.InitialDelayMilliseconds) * time.Millisecond)
	logger.Debug("delay set", "now", now)
	logger.Debug("delay set", "initialDelayTime", initialDelayTime)

	ticker.Reset(time.Duration(egressConfig.PeriodMilliseconds) * time.Millisecond)
	logger.Info("config update processing complete")
}

func performProbes(tickTime *time.Time) {
	logger.Debug("tick received", "tickTime", tickTime)

	for _, probe := range probes {
		// TODO-HIGH: add the other probes here
		// for how Kubernetes tests their GRPC probe: https://pkg.go.dev/k8s.io/kubernetes/test/images/agnhost#readme-grpc-health-checking
		if probe.HTTPGetAction != nil {
			probe.HTTPGetAction.act(probe.AttemptsMetric, probe.ResponseTimeMetric)
		}
	}
}
