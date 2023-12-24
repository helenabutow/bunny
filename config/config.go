package config

import (
	"crypto/sha256"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"sync"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

type BunnyConfig struct {
	IngressConfig IngressConfig `yaml:"ingress"`
	SignalsConfig SignalsConfig `yaml:"signals"`
	OTelConfig    OTelConfig    `yaml:"otel"`
}

type IngressConfig struct {
	Port int    `yaml:"port"`
	Path string `yaml:"path"`
}

type SignalsConfig struct {
	WatchedProcessName *string `yaml:"watchedProcessName"`
}

type OTelConfig struct {
	MeterName string `yaml:"meterName"`
}

const defaultConfigFilePath string = "/config/bunny.yaml"

var logger *slog.Logger = nil
var configDirPath string = path.Dir(defaultConfigFilePath)
var configFilePath string = defaultConfigFilePath
var bunnyConfig *BunnyConfig = nil
var configUpdateChannels []chan BunnyConfig = []chan BunnyConfig{}
var OSSignalsChannel chan os.Signal = make(chan os.Signal, 1)

func Init(sharedLogger *slog.Logger) {
	logger = sharedLogger
	logger.Info("Config initializing")
	logger.Info("Config is initialized")
}

func GoConfig(wg *sync.WaitGroup) {
	defer wg.Done()

	logger.Info("Config is go!")

	// figure out where to read the config file from
	configFilePathEnvVar := os.Getenv("BUNNY_CONFIG_FILE_PATH")
	if configFilePathEnvVar != "" {
		logger.Info("config file path set via env var", "BUNNY_CONFIG_FILE_PATH", configFilePathEnvVar)
		configDirPath = path.Dir(configFilePathEnvVar)
		configFilePath = configFilePathEnvVar
	}

	readBunnyConfigFile()

	// show the config being used
	logConfigBeingUsed()

	// notify of first config via channel
	for _, configUpdateChannel := range configUpdateChannels {
		configUpdateChannel <- *bunnyConfig
	}

	// create file watcher for config file
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Error("could not create watcher for config file", "err", err)
	} else {
		defer watcher.Close()
		// we watch the directory instead of the file because of https://github.com/fsnotify/fsnotify#watching-a-file-doesnt-work-well
		err = watcher.Add(configDirPath)
		if err != nil {
			logger.Error("couldn't create a watcher for the directory. Continuing with default config", "err", err)
		}
	}

	// wait for messages
	var configFileHash [sha256.Size]byte = sha256.Sum256([]byte(""))
	for {
		select {
		// wait for config file changes or for the config file to be created
		case event, ok := <-watcher.Events:
			if !ok {
				logger.Error("watcher closed for events")
				continue
			}
			logger.Debug("event=" + event.String())
			// we're only interested in files being written or created
			// since we aren't watching for files being deleted, this results in bunny keeping the existing config
			// if a file is deleted. This is intentional, to ensure that if the config file will be quickly replaced
			// with a new one, that bunny doesn't (briefly) revert to the default config
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				logger.Debug("file in dir of config file been written or created")
			}

			// rather than try to handle all the various way in which a file can be replaced on various platforms,
			// we instead just check for changes in the file hash. This is slower but much simpler to implement.
			data, err := os.ReadFile(configFilePath)
			if err != nil {
				logger.Error("could not generate hash of config file")
			}
			newConfigFileHash := sha256.Sum256(data)
			if newConfigFileHash != configFileHash {
				logger.Info("bunny config content has changed")
				readBunnyConfigFile()
				logger.Debug("after reading the config file", "configFileHash", configFileHash)
				logger.Debug("after reading the config file", "newConfigFileHash", newConfigFileHash)
				configFileHash = newConfigFileHash
				// show the config being used
				logConfigBeingUsed()
				// notify of config change via channel
				for _, configUpdateChannel := range configUpdateChannels {
					configUpdateChannel <- *bunnyConfig
				}
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				logger.Error("watcher closed for errors")
				continue
			}
			logger.Error("error while watching config file: ", err)

		case signal, ok := <-OSSignalsChannel:
			if !ok {
				logger.Error("could not process signal from signal channel")
			}
			logger.Info("received signal. Ending go routine.", "signal", signal)
			return
		}
	}
}

func AddChannelListener(configUpdateChannel *(chan BunnyConfig)) {
	configUpdateChannels = append(configUpdateChannels, *configUpdateChannel)
}

func generateDefaultConfig() *BunnyConfig {
	return &BunnyConfig{
		IngressConfig: IngressConfig{
			Port: 1312,
			Path: "healthz",
		},
		OTelConfig: OTelConfig{
			MeterName: "bunny",
		},
	}
}

func readBunnyConfigFile() {
	// reset the config to the defaults first (in case the config file doesn't exist)
	bunnyConfig = generateDefaultConfig()
	// read the config file (checking if it exists)
	data, err := os.ReadFile(configFilePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			logger.Error("bunny config file does not exist at \"" + configFilePath + "\". Continuing with default config")
		} else {
			logger.Error("error while reading the bunny config file. Continuing with default config", "err", err)
		}
		return
	}

	// convert the YAML into a struct
	err = yaml.Unmarshal([]byte(data), &bunnyConfig)
	if err != nil {
		logger.Error("could not unmarshal data for bunny config file (continuing with default config)", "err", err)
	}
}

func logConfigBeingUsed() {
	data, err := yaml.Marshal(&bunnyConfig)
	if err != nil {
		logger.Error("cannot marshal data", "err", err)
	}
	logger.Info("using config", "data", string(data))
}
