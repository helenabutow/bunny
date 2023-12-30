package config

// TODO-MEDIUM: support millisecond periods, timeouts, and delays (mainly so that period and timeout can be less than 1 second)
// TODO-LOW: add support for GRPC, TCP, and exec probes
// TODO-LOW: when we implement exec probes, do we want to wrap it in https://github.com/equinix-labs/otel-cli ?
type EgressConfig struct {
	HTTPGetActionConfig    *HTTPGetActionConfig   `yaml:"httpGet"`
	InitialDelaySeconds    int                    `yaml:"initialDelaySeconds"`
	PeriodSeconds          int                    `yaml:"periodSeconds"`
	TimeoutSeconds         int                    `yaml:"timeoutSeconds"`
	EgressPrometheusConfig EgressPrometheusConfig `yaml:"prometheus"`
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

type EgressPrometheusConfig struct {
	ExtraEgressPrometheusLabels []ExtraEgressPrometheusLabelsConfig `yaml:"extraLabels"`
	MetricsEnabled              []string                            `yaml:"metricsEnabled"`
}

type ExtraEgressPrometheusLabelsConfig struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}
