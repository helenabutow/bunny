package signals

import (
	"bunny/config"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	// TODO-LOW: find a better package for getting a list of processes since mitchellh/go-ps
	// is no longer maintained and it doesn't give us cmd args or the env to filter on
	"github.com/mitchellh/go-ps"
)

var logger *slog.Logger = nil
var osSignalsChannel chan os.Signal = make(chan os.Signal, 1)
var osSignalListenerChannels []chan os.Signal = []chan os.Signal{}
var ConfigUpdateChannel chan config.BunnyConfig = make(chan config.BunnyConfig, 1)
var signalsConfig *config.SignalsConfig = nil

func Init(sharedLogger *slog.Logger) {
	logger = sharedLogger
	logger.Info("Signals initializing")
	// since we may need to wait for a process to die before exiting,
	// we register a single channel here with os/signal, then in
	// GoSignal, we wait for it and for the process to die before
	// forwarding it further to the other channels
	signal.Notify(osSignalsChannel, syscall.SIGINT, syscall.SIGTERM)
	logger.Info("Signals is initialized")
}

func AddChannelListener(listenersChannel *(chan os.Signal)) {
	osSignalListenerChannels = append(osSignalListenerChannels, *listenersChannel)
}

func GoSignals(wg *sync.WaitGroup) {
	defer wg.Done()

	logger.Info("Signals is go!")

	for {
		logger.Info("waiting for config or signal")
		select {
		case bunnyConfig, ok := <-ConfigUpdateChannel:
			if !ok {
				continue
			}
			logger.Info("received config update")
			signalsConfig = &bunnyConfig.SignalsConfig
			logger.Info("config update processing complete")

		case signal, ok := <-osSignalsChannel:
			if !ok {
				logger.Error("could not process signal from signal channel")
			}
			logger.Info("received signal", "signal", signal)
			if signalsConfig != nil &&
				signalsConfig.WatchedProcessName != nil &&
				*signalsConfig.WatchedProcessName != "" {
				logger.Info("checking for process to watch")
				for processExists := true; processExists; {
					processExists = false

					// get the list of processes
					logger.Info("getting the list of processes")
					processes, err := ps.Processes()
					if err != nil {
						logger.Error("could not get list of processes", "err", err)
					}

					// check if any of the processes match the regex
					for _, process := range processes {
						if *signalsConfig.WatchedProcessName == process.Executable() {
							logger.Info("found process to wait on")
							logger.Debug("found process to wait on", "process.Executable()", process.Executable())
							logger.Debug("found process to wait on", "process.Pid()", process.Pid())
							processExists = true
							break
						}
					}

					// sleep so that we don't hammer /proc
					if processExists {
						logger.Info("sleeping for a second before checking again")
						time.Sleep(1 * time.Second)
					} else {
						logger.Info("process not found")
					}
				}
			}
			logger.Info("notifying packages of signal", "signal", signal)
			for _, listenerChannel := range osSignalListenerChannels {
				listenerChannel <- signal
			}
			logger.Info("ending go routine")
			return
		}
	}
}
