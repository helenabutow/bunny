package config

// TODO-LOW: add support for GRPC, TCP, and exec probes
// TODO-LOW: when we implement exec probes, do we want to wrap it in https://github.com/equinix-labs/otel-cli ?
type EgressConfig struct {
	Probes                   []EgressProbeConfig `yaml:"probes"`
	InitialDelayMilliseconds int                 `yaml:"initialDelayMilliseconds"`
	PeriodMilliseconds       int                 `yaml:"periodMilliseconds"`
	TimeoutMilliseconds      int                 `yaml:"timeoutMilliseconds"`
}

type EgressProbeConfig struct {
	Name      string                   `yaml:"name"`
	Metrics   EgressProbeMetricsConfig `yaml:"metrics"`
	GRPC      *GRPCActionConfig        `yaml:"grpc"`
	HTTPGet   *HTTPGetActionConfig     `yaml:"httpGet"`
	TCPSocket *TCPSocketActionConfig   `yaml:"tcpSocket"`
}

type EgressProbeMetricsConfig struct {
	Attempts     MetricsConfig `yaml:"attempts"`
	ResponseTime MetricsConfig `yaml:"responseTime"`
}

type GRPCActionConfig struct {
	Port    int     `yaml:"port"`
	Service *string `yaml:"service"`
}

type HTTPGetActionConfig struct {
	Host        *string             `yaml:"host"`
	HTTPHeaders []HTTPHeadersConfig `yaml:"httpHeaders"`
	Port        int                 `yaml:"port"`
	Path        string              `yaml:"path"`
}

type HTTPHeadersConfig struct {
	Name  string   `yaml:"name"`
	Value []string `yaml:"value"`
}

type TCPSocketActionConfig struct {
	Port   int             `yaml:"port"`
	Host   *string         `yaml:"service"`
	Expect *[]ExpectConfig `yaml:"expect"`
}

type ExpectConfig struct {
	Send    *SendStepConfig    `yaml:"send"`
	Receive *ReceiveStepConfig `yaml:"receive"`
}

type SendStepConfig struct {
	Text      string `yaml:"text"`
	Delimiter string `yaml:"delimiter"`
}

type ReceiveStepConfig struct {
	RegEx     string `yaml:"regex"`
	Delimiter string `yaml:"delimiter"`
}
