package signals

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

var logger *log.Logger = log.Default()

// var osSignalsChannel chan os.Signal = make(chan os.Signal, 1)

func Init() {
	logger.Println("Signals initializing")
	logger.Println("Signals is initialized")
}

func AddChannelListener(signalChannel *(chan os.Signal)) {
	// this works but if we want to delay until the app process exits, we'll need to:
	// 1. create a go thread (like for config and ingress)
	// 2. create a new AddChannelListener func (like for config)
	// 3. listen for the signal in the go thread, wait for the app process to exit, and then broadcast a new message to all the listeners
	signal.Notify(*signalChannel, syscall.SIGINT, syscall.SIGTERM)
}

// TODO-HIGH: do we wait until the app process exits? (since we should be in the same PID namespace, does that seem reasonable?)
