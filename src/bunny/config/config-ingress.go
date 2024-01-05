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

// TODO-MEDIUM: we should remove the "query" prefix on these names
type InstantQueryConfig struct {
	QueryTimeout             string `yaml:"queryTimeout"`
	QueryRelativeInstantTime string `yaml:"queryRelativeInstantTime"`
	Query                    string `yaml:"query"`
}

type RangeQueryConfig struct {
	QueryTimeout           string `yaml:"queryTimeout"`
	QueryRelativeStartTime string `yaml:"queryRelativeStartTime"`
	QueryRelativeEndTime   string `yaml:"queryRelativeEndTime"`
	QueryInterval          string `yaml:"queryInterval"`
	Query                  string `yaml:"query"`
}

type IngressPrometheusConfig struct {
	ExtraIngressPrometheusLabels []ExtraIngressPrometheusLabelsConfig `yaml:"extraLabels"`
	MetricsEnabled               []string                             `yaml:"metricsEnabled"`
}

type ExtraIngressPrometheusLabelsConfig struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}
