package config

// TODO-LOW: add support for GRPC, TCP, and exec probes
// TODO-LOW: when we implement exec probes, do we want to wrap it in https://github.com/equinix-labs/otel-cli ?
type EgressConfig struct {
	HTTPGetActionConfig *HTTPGetActionConfig `yaml:"httpGet"`
	InitialDelaySeconds int                  `yaml:"initialDelaySeconds"`
	PeriodSeconds       int                  `yaml:"periodSeconds"`
	TimeoutSeconds      int                  `yaml:"timeoutSeconds"`
	PrometheusConfig    PrometheusConfig     `yaml:"prometheus"`
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

type PrometheusConfig struct {
	ExtraPrometheusLabels []ExtraPrometheusLabelsConfig `yaml:"extraLabels"`
	MetricsEnabled        []string                      `yaml:"metricsEnabled"`
}

type ExtraPrometheusLabelsConfig struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}
