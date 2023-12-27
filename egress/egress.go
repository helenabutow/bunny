package egress

import (
	"bunny/config"
	"bunny/otel"
	"context"
	"log/slog"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	api "go.opentelemetry.io/otel/metric"
)

var logger *slog.Logger = nil
var ConfigUpdateChannel chan config.BunnyConfig = make(chan config.BunnyConfig, 1)
var OSSignalsChannel chan os.Signal = make(chan os.Signal, 1)
var ticker *time.Ticker = nil
var egressConfig *config.EgressConfig = nil
var meter *api.Meter = nil
var extraAttributes *api.MeasurementOption = nil
var probeAttempts int64 = 0
var probeAttemptsCounter *api.Int64Counter = nil

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
		logger.Info("waiting for config or signal")
		select {
		case tickTime := <-ticker.C:
			logger.Debug("tick received", "tickTime", tickTime)

			probeAttempts++
			logger.Debug("probeAttempts has increased", "probeAttempts", probeAttempts)
			(*probeAttemptsCounter).Add(context.Background(), 1, *extraAttributes)

			// TODO-HIGH: implement performing the probe and collecting the metrics for it

		case bunnyConfig, ok := <-ConfigUpdateChannel:
			if !ok {
				continue
			}
			logger.Info("received config update")
			egressConfig = &bunnyConfig.EgressConfig
			meter = otel.Meter

			// TODO-LOW: replace these attributes with ones read from config
			// extra key/value pairs to include with each Prometheus metric
			newExtraAttributes := api.WithAttributes(
				attribute.Key("A").String("B"),
				attribute.Key("C").String("D"),
			)
			extraAttributes = &newExtraAttributes

			// TODO-LOW: each metric that egress generates should toggle-able
			// if someone doesn't need a metric, we shouldn't waste cpu generating values for it
			// and if they're opt-in, scrape configs get simpler and don't have to change as metrics are added/removed
			// (which would be a pain if someone was using annotation based scrape configs on their Pods)
			newProbeAttemptsCounter, err := (*meter).Int64Counter("egress_probe_attempts", api.WithDescription("the number of probes attempted"))
			if err != nil {
				logger.Error("could not create probeAttemptsCounter", "err", err)
			}
			probeAttemptsCounter = &newProbeAttemptsCounter

			ticker.Reset(time.Duration(egressConfig.PeriodSeconds) * time.Second)
			logger.Info("config update processing complete")

		case signal, ok := <-OSSignalsChannel:
			if !ok {
				logger.Error("could not process signal from signal channel")
			}
			logger.Info("received signal. Ending go routine.", "signal", signal)
			return
		}

	}
}
