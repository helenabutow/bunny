package config

import (
	"bunny/logging"
	"crypto/sha256"
	"encoding/hex"
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
	Egress    EgressConfig    `yaml:"egress"`
	Ingress   IngressConfig   `yaml:"ingress"`
	Signals   SignalsConfig   `yaml:"signals"`
	Telemetry TelemetryConfig `yaml:"telemetry"`
}

// EgressConfig is in config-egress.go
// IngressConfig is in config-ingress.go
// config common to both is here

type MetricsConfig struct {
	Enabled     bool                `yaml:"enabled"`
	Name        string              `yaml:"name"`
	ExtraLabels []ExtraLabelsConfig `yaml:"extraLabels"`
}

type ExtraLabelsConfig struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

// TelemetryConfig is in config-telemetry.go

type SignalsConfig struct {
	WatchedProcessCommandLineRegEx *string `yaml:"watchedProcessCommandLineRegEx"`
}

type ConfigStage int

const ConfigStageTelemetryCompleted = 1

const defaultConfigFilePath string = "/config/bunny.yaml"

var logger *slog.Logger = nil
var configDirPath string = path.Dir(defaultConfigFilePath)
var configFilePath string = defaultConfigFilePath
var bunnyConfig *BunnyConfig = nil
var configUpdateChannels []chan BunnyConfig = []chan BunnyConfig{}
var OSSignalsChannel chan os.Signal = make(chan os.Signal, 1)

func GoConfig(wg *sync.WaitGroup) {
	defer wg.Done()

	logger = logging.ConfigureLogger("config")
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
	watcher, err := fsnotify.NewBufferedWatcher(100)
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
	var configFileHash string = ""
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
			hash := sha256.New()
			hash.Write(data)
			newConfigFileHash := hex.EncodeToString(hash.Sum(nil))
			if newConfigFileHash != configFileHash {
				logger.Info("bunny config content has changed")
				readBunnyConfigFile()
				logger.Debug("after reading the config file", "data", string(data))
				logger.Debug("after reading the config file", "configFileHash", configFileHash)
				logger.Debug("after reading the config file", "newConfigFileHash", newConfigFileHash)
				configFileHash = newConfigFileHash
				// show the config being used
				logConfigBeingUsed()

				// TODO-HIGH: check if the config is valid before applying it
				// test for unknown keys in the YAML (which I think the yaml package should error on)
				// and for incorrect types (e.g. strings instead of integers)
				// also test the values (e.g. empty strings)
				// we will also want to check this when loading the file for the first time above

				// TODO-HIGH: add the ability to do a random delay before consuming the updated config
				// * this should support a range (e.g. "5s", "15m", "4h", "2d")
				// * the wait period used should come from the config being loaded (with fall back to the one that exists)
				//   this way if an emergency change needs to be made, it can be applied without waiting for the old change period to complete
				// TODO-HIGH: if the config is updated during the wait period, what do we do?

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
			logger.Info("completed shutdowns. Returning from go routine")
			return
		}
	}
}

func AddChannelListener(configUpdateChannel *(chan BunnyConfig)) {
	configUpdateChannels = append(configUpdateChannels, *configUpdateChannel)
}

// TODO-MEDIUM: we need a better default config
func generateDefaultConfig() *BunnyConfig {
	return &BunnyConfig{
		Ingress: IngressConfig{
			HTTPServerConfig: HTTPServerConfig{
				Port:                          1312,
				ReadTimeoutMilliseconds:       5000,
				ReadHeaderTimeoutMilliseconds: 5000,
				WriteTimeoutMilliseconds:      10000,
				IdleTimeoutMilliseconds:       2000,
				MaxHeaderBytes:                10000,
				OpenTelemetryMetricsPath:      "otel-metrics",
				PrometheusMetricsPath:         "prom-metrics",
			},
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
