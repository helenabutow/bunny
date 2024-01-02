package config

// TODO-LOW: add support for GRPC, TCP, and exec probes
// TODO-LOW: when we implement exec probes, do we want to wrap it in https://github.com/equinix-labs/otel-cli ?
type EgressConfig struct {
	EgressProbeConfigs       []EgressProbeConfig `yaml:"probes"`
	InitialDelayMilliseconds int                 `yaml:"initialDelayMilliseconds"`
	PeriodMilliseconds       int                 `yaml:"periodMilliseconds"`
	TimeoutMilliseconds      int                 `yaml:"timeoutMilliseconds"`
}

type EgressProbeConfig struct {
	Name                     string                   `yaml:"name"`
	EgressProbeMetricsConfig EgressProbeMetricsConfig `yaml:"metrics"`
	HTTPGetActionConfig      *HTTPGetActionConfig     `yaml:"httpGet"`
}

type EgressProbeMetricsConfig struct {
	Attempts     EgressMetricsConfig `yaml:"attempts"`
	ResponseTime EgressMetricsConfig `yaml:"responseTime"`
}

type EgressMetricsConfig struct {
	Enabled                  bool                            `yaml:"enabled"`
	Name                     string                          `yaml:"name"`
	EgressMetricsExtraLabels []EgressMetricsExtraLabelConfig `yaml:"extraLabels"`
}

type EgressMetricsExtraLabelConfig struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

type HTTPGetActionConfig struct {
	Host        *string             `yaml:"host"`
	HTTPHeaders []HTTPHeadersConfig `yaml:"httpHeaders"`
	Port        int                 `yaml:"port"`
	Path        string              `yaml:"path"`
}

type HTTPHeadersConfig struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}
