package config

type IngressConfig struct {
	HTTPServerConfig HTTPServerConfig `yaml:"httpServer"`
}

type HTTPServerConfig struct {
	Port                          int            `yaml:"port"`
	ReadTimeoutMilliseconds       int            `yaml:"readTimeoutMilliseconds"`
	ReadHeaderTimeoutMilliseconds int            `yaml:"readHeaderTimeoutMilliseconds"`
	WriteTimeoutMilliseconds      int            `yaml:"writeTimeoutMilliseconds"`
	IdleTimeoutMilliseconds       int            `yaml:"idleTimeoutMilliseconds"`
	MaxHeaderBytes                int            `yaml:"maxHeaderBytes"`
	OpenTelemetryMetricsPath      string         `yaml:"openTelemetryMetricsPath"`
	PrometheusMetricsPath         string         `yaml:"prometheusMetricsPath"`
	Health                        []HealthConfig `yaml:"health"`
}

type HealthConfig struct {
	Path         string                              `yaml:"path"`
	InstantQuery *InstantQueryConfig                 `yaml:"instantQuery"`
	RangeQuery   *RangeQueryConfig                   `yaml:"rangeQuery"`
	Metrics      *IngressHealthEndpointMetricsConfig `yaml:"metrics"`
}

type InstantQueryConfig struct {
	Timeout             string `yaml:"timeout"`
	RelativeInstantTime string `yaml:"relativeInstantTime"`
	Query               string `yaml:"query"`
}

type RangeQueryConfig struct {
	Timeout           string `yaml:"timeout"`
	RelativeStartTime string `yaml:"relativeStartTime"`
	RelativeEndTime   string `yaml:"relativeEndTime"`
	Interval          string `yaml:"interval"`
	Query             string `yaml:"query"`
}

type IngressHealthEndpointMetricsConfig struct {
	Attempts     MetricsConfig `yaml:"attempts"`
	ResponseTime MetricsConfig `yaml:"responseTime"`
	Successes    MetricsConfig `yaml:"successes"`
}
