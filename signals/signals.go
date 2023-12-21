package signals

import (
	"bunny/config"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	// TODO-LOW: find a better package for getting a list of processes since mitchellh/go-ps
	// is no longer maintained and it doesn't give us cmd args or the env to filter on
	"github.com/mitchellh/go-ps"
)

var logger *log.Logger = log.Default()

var osSignalsChannel chan os.Signal = make(chan os.Signal, 1)
var osSignalListenerChannels []chan os.Signal = []chan os.Signal{}
var ConfigUpdateChannel chan config.BunnyConfig = make(chan config.BunnyConfig, 1)
var signalsConfig *config.SignalsConfig = nil

func Init() {
	logger.Println("Signals initializing")
	// since we may need to wait for a process to die before exiting,
	// we register a single channel here with os/signal, then in
	// GoSignal, we wait for it and for the process to die before
	// forwarding it further to the other channels
	signal.Notify(osSignalsChannel, syscall.SIGINT, syscall.SIGTERM)
	logger.Println("Signals is initialized")
}

func AddChannelListener(listenersChannel *(chan os.Signal)) {
	osSignalListenerChannels = append(osSignalListenerChannels, *listenersChannel)
}

func GoSignals(wg *sync.WaitGroup) {
	defer wg.Done()

	logger.Println("Signals is go!")

	for {
		logger.Println("waiting for config or signal")
		select {
		case bunnyConfig, ok := <-ConfigUpdateChannel:
			if !ok {
				continue
			}
			logger.Println("received config update")
			signalsConfig = &bunnyConfig.SignalsConfig
			logger.Println("config update processing complete")

		case signal, ok := <-osSignalsChannel:
			if !ok {
				logger.Println("could not process signal from signal channel")
			}
			logger.Printf("received signal %v", signal)
			if signalsConfig != nil &&
				signalsConfig.WatchedProcessName != nil &&
				*signalsConfig.WatchedProcessName != "" {
				logger.Printf("checking for process to watch")
				for processExists := true; processExists; {
					processExists = false

					// get the list of processes
					logger.Print("getting the list of processes")
					processes, err := ps.Processes()
					if err != nil {
						logger.Printf("could not get list of processes: %v", err)
					}

					// check if any of the processes match the regex
					for _, process := range processes {
						if *signalsConfig.WatchedProcessName == process.Executable() {
							logger.Printf("found process to wait on")
							logger.Printf("process.Executable() = \"%v\"", process.Executable())
							logger.Printf("process.Pid() = \"%v\"", process.Pid())
							processExists = true
							break
						}
					}

					// sleep so that we don't hammer /proc
					if processExists {
						logger.Printf("sleeping for a second before checking again")
						time.Sleep(1 * time.Second)
					} else {
						logger.Printf("process not found")
					}
				}
			}
			logger.Printf("notifying packages of signal %v", signal)
			for _, listenerChannel := range osSignalListenerChannels {
				listenerChannel <- signal
			}
			logger.Printf("ending go routine")
			return
		}
	}
}
