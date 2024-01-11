package egress

import (
	"bunny/config"
	"bunny/logging"
	"log/slog"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var logger *slog.Logger = nil
var ConfigUpdateChannel chan config.BunnyConfig = make(chan config.BunnyConfig, 1)
var OSSignalsChannel chan os.Signal = make(chan os.Signal, 1)
var ConfigStageChannel chan config.ConfigStage = make(chan config.ConfigStage, 1)
var ticker *time.Ticker = nil
var initialDelayTime time.Time = time.Now()
var egressConfig *config.EgressConfig = nil
var probes []Probe = []Probe{}
var meter *metric.Meter = nil
var tracer *trace.Tracer = nil

func GoEgress(wg *sync.WaitGroup) {
	defer wg.Done()

	logger = logging.ConfigureLogger("egress")
	logger.Info("Egress is go!")

	// yes, this looks silly
	// the goal here is to prevent the ticker from firing until config has been loaded
	ticker = time.NewTicker(1 * time.Hour)
	ticker.Stop()

	for {
		logger.Debug("waiting for config or signal")
		select {
		case tickTime := <-ticker.C:
			if initialDelayTime.Before(time.Now()) {
				performProbes(&tickTime)
			}

		case bunnyConfig, ok := <-ConfigUpdateChannel:
			if !ok {
				logger.Error("could not process config from config update channel")
				continue
			}
			updateConfig(&bunnyConfig)

		case signal, ok := <-OSSignalsChannel:
			if !ok {
				logger.Error("could not process signal from signal channel")
			}
			logger.Info("received signal. Ending go routine.", "signal", signal)
			logger.Info("completed shutdowns. Returning from go routine")
			return
		}
	}
}

func updateConfig(bunnyConfig *config.BunnyConfig) {
	logger.Info("received config update")
	egressConfig = &bunnyConfig.Egress

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

	newMeter := otel.GetMeterProvider().Meter("bunny/egress")
	meter = &newMeter
	newTracer := otel.GetTracerProvider().Tracer("bunny/egress")
	tracer = &newTracer
	otel.SetTextMapPropagator(propagation.TraceContext{})

	// process probe configs
	probes = []Probe{}
	timeout := time.Duration(egressConfig.TimeoutMilliseconds) * time.Millisecond
	for _, egressProbeConfig := range egressConfig.Probes {
		var newProbe *Probe = newProbe(&egressProbeConfig, timeout)
		probes = append(probes, *newProbe)
	}

	now := time.Now()
	initialDelayTime = now.Add(time.Duration(egressConfig.InitialDelayMilliseconds) * time.Millisecond)
	logger.Debug("delay set", "now", now)
	logger.Debug("delay set", "initialDelayTime", initialDelayTime)

	if egressConfig.PeriodMilliseconds > 0 {
		ticker.Reset(time.Duration(egressConfig.PeriodMilliseconds) * time.Millisecond)
	} else {
		ticker.Stop()
	}
	logger.Info("config update processing complete")
}

func performProbes(tickTime *time.Time) {
	logger.Debug("tick received", "tickTime", tickTime)

	for _, probe := range probes {
		if probe.ProbeAction != nil {
			probeAction := *probe.ProbeAction
			probeAction.act(probe.AttemptsMetric, probe.ResponseTimeMetric)
		}
	}
}
