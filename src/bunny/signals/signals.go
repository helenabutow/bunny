package signals

import (
	"bunny/config"
	"bunny/logging"
	"log/slog"
	"os"
	"os/signal"
	"regexp"
	"sync"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/process"
)

var logger *slog.Logger = nil
var osSignalsChannel chan os.Signal = make(chan os.Signal, 1)
var osSignalListenerChannels []chan os.Signal = []chan os.Signal{}
var ConfigUpdateChannel chan config.BunnyConfig = make(chan config.BunnyConfig, 1)
var signalsConfig *config.SignalsConfig = nil
var watchedProcessCommandLineRegEx *regexp.Regexp = nil

func AddChannelListener(listenersChannel *(chan os.Signal)) {
	osSignalListenerChannels = append(osSignalListenerChannels, *listenersChannel)
}

func GoSignals(wg *sync.WaitGroup) {
	defer wg.Done()

	logger = logging.ConfigureLogger("signals")
	logger.Info("Signals is go!")

	// since we may need to wait for a process to die before exiting,
	// we register a single channel here with os/signal, then we wait
	// for it and for the process to die before forwarding it further
	// to the other channels
	signal.Notify(osSignalsChannel, syscall.SIGINT, syscall.SIGTERM)

	for {
		logger.Debug("waiting for config or signal")
		select {
		case bunnyConfig, ok := <-ConfigUpdateChannel:
			if !ok {
				logger.Error("could not process config from config update channel")
				continue
			}
			logger.Info("received config update")
			signalsConfig = &bunnyConfig.Signals
			var err error
			watchedProcessCommandLineRegEx, err = regexp.Compile(*signalsConfig.WatchedProcessCommandLineRegEx)
			if err != nil {
				logger.Error("could not compile watched process regex in config file")
				continue
			}
			logger.Info("config update processing complete")

		case signal, ok := <-osSignalsChannel:
			if !ok {
				logger.Error("could not process signal from signal channel")
			}
			logger.Info("received signal", "signal", signal)
			waitForProcess()
			logger.Info("notifying packages of signal", "signal", signal)
			for _, listenerChannel := range osSignalListenerChannels {
				listenerChannel <- signal
			}
			logger.Info("completed shutdowns. Returning from go routine")
			return
		}
	}
}

func waitForProcess() {
	logger.Info("checking for process to watch")

	if watchedProcessCommandLineRegEx == nil {
		logger.Info("no config for watching a process")
		return
	}

	for processExists := true; processExists; {
		processExists = false

		// get the list of processes
		logger.Info("getting the list of processes")
		processes, err := process.Processes()
		if err != nil {
			logger.Error("could not get list of processes", "err", err)
		}

		// check if any of the processes match the regex
		for _, process := range processes {
			commandLine, err := process.Cmdline()
			if err != nil {
				logger.Error("could not get command line for process", "process", process)
			}
			logger.Debug("checking command line", "commandLine", commandLine)
			if watchedProcessCommandLineRegEx.Find([]byte(commandLine)) != nil {
				logger.Info("found process to wait on",
					"process.Pid", process.Pid,
					"commandLine", commandLine)
				processExists = true
				break
			}
		}

		// sleep so that we don't hammer /proc or the kernel
		if processExists {
			logger.Info("sleeping for a second before checking again")
			time.Sleep(1 * time.Second)
		} else {
			logger.Info("process not found")
		}
	}
}
