package egress

import (
	"bunny/config"
	"log/slog"
	"os"
	"sync"
	"time"
)

var logger *slog.Logger = nil
var ConfigUpdateChannel chan config.BunnyConfig = make(chan config.BunnyConfig, 1)
var OSSignalsChannel chan os.Signal = make(chan os.Signal, 1)
var ticker *time.Ticker = nil
var egressConfig *config.EgressConfig = nil

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

		case bunnyConfig, ok := <-ConfigUpdateChannel:
			if !ok {
				continue
			}
			logger.Info("received config update")
			egressConfig = &bunnyConfig.EgressConfig
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