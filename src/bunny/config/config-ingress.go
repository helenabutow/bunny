package config

type IngressConfig struct {
	HTTPServerConfig        HTTPServerConfig        `yaml:"httpServer"`
	IngressPrometheusConfig IngressPrometheusConfig `yaml:"prometheus"`
}

type HTTPServerConfig struct {
	Port                          int    `yaml:"port"`
	HealthPath                    string `yaml:"healthPath"`
	MetricsPath                   string `yaml:"metricsPath"`
	ReadTimeoutMilliseconds       int    `yaml:"readTimeoutMilliseconds"`
	ReadHeaderTimeoutMilliseconds int    `yaml:"readHeaderTimeoutMilliseconds"`
	WriteTimeoutMilliseconds      int    `yaml:"writeTimeoutMilliseconds"`
	IdleTimeoutMilliseconds       int    `yaml:"idleTimeoutMilliseconds"`
	MaxHeaderBytes                int    `yaml:"maxHeaderBytes"`
}

type IngressPrometheusConfig struct {
	ExtraIngressPrometheusLabels []ExtraIngressPrometheusLabelsConfig `yaml:"extraLabels"`
	MetricsEnabled               []string                             `yaml:"metricsEnabled"`
}

type ExtraIngressPrometheusLabelsConfig struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}
