package config

type TelemetryConfig struct {
	OpenTelemetry OpenTelemetryConfig `yaml:"openTelemetry"`
	Prometheus    PrometheusConfig    `yaml:"prometheus"`
}

type OpenTelemetryConfig struct {
	Exporters []string `yaml:"exporters"`
}

type PrometheusConfig struct {
	TSDBPath    string              `yaml:"tsdbPath"`
	TSDBOptions TSDBOptionsConfig   `yaml:"tsdbOptions"`
	PromQL      PromQLOptionsConfig `yaml:"promql"`
}

type TSDBOptionsConfig struct {
	RetentionDurationMilliseconds int `yaml:"retentionDurationMilliseconds"`
	MinBlockDurationMilliseconds  int `yaml:"minBlockDurationMilliseconds"`
	MaxBlockDurationMilliseconds  int `yaml:"maxBlockDurationMilliseconds"`
}

type PromQLOptionsConfig struct {
	MaxConcurrentQueries int                 `yaml:"maxConcurrentQueries"`
	EngineOptions        EngineOptionsConfig `yaml:"engineOptions"`
}

type EngineOptionsConfig struct {
	MaxSamples                         int `yaml:"maxSamples"`
	TimeoutMilliseconds                int `yaml:"timeoutMilliseconds"`
	LookbackDeltaMilliseconds          int `yaml:"lookbackDeltaMilliseconds"`
	NoStepSubqueryIntervalMilliseconds int `yaml:"noStepSubqueryIntervalMilliseconds"`
}
