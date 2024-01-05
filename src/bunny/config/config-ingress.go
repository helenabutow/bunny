package config

type IngressConfig struct {
	HTTPServerConfig        HTTPServerConfig        `yaml:"httpServer"`
	IngressPrometheusConfig IngressPrometheusConfig `yaml:"prometheus"`
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
	HealthConfig                  []HealthConfig `yaml:"health"`
}

type HealthConfig struct {
	Path               string              `yaml:"path"`
	InstantQueryConfig *InstantQueryConfig `yaml:"instantQuery"`
	RangeQueryConfig   *RangeQueryConfig   `yaml:"rangeQuery"`
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

type IngressPrometheusConfig struct {
	ExtraIngressPrometheusLabels []ExtraIngressPrometheusLabelsConfig `yaml:"extraLabels"`
	MetricsEnabled               []string                             `yaml:"metricsEnabled"`
}

type ExtraIngressPrometheusLabelsConfig struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}
