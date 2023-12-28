package config

type IngressConfig struct {
	HTTPServerConfig        HTTPServerConfig        `yaml:"httpServer"`
	IngressPrometheusConfig IngressPrometheusConfig `yaml:"prometheus"`
}

type HTTPServerConfig struct {
	Port              int    `yaml:"port"`
	HealthPath        string `yaml:"healthPath"`
	MetricsPath       string `yaml:"metricsPath"`
	ReadTimeout       int    `yaml:"readTimeout"`
	ReadHeaderTimeout int    `yaml:"readHeaderTimeout"`
	WriteTimeout      int    `yaml:"writeTimeout"`
	IdleTimeout       int    `yaml:"idleTimeout"`
	MaxHeaderBytes    int    `yaml:"maxHeaderBytes"`
}

type IngressPrometheusConfig struct {
	ExtraIngressPrometheusLabels []ExtraIngressPrometheusLabelsConfig `yaml:"extraLabels"`
	MetricsEnabled               []string                             `yaml:"metricsEnabled"`
}

type ExtraIngressPrometheusLabelsConfig struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}
