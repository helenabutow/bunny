package egress

import (
	"bunny/config"
	"bunny/telemetry"
	"time"
)

type Probe struct {
	Name               string
	AttemptsMetric     *telemetry.AttemptsMetric
	ResponseTimeMetric *telemetry.ResponseTimeMetric
	ProbeAction        *ProbeAction
}

type ProbeAction interface {
	act(probeName string, attemptsMetric *telemetry.AttemptsMetric, responseTimeMetric *telemetry.ResponseTimeMetric)
}

func newProbe(egressProbeConfig *config.EgressProbeConfig, timeout time.Duration) *Probe {
	var probeAction ProbeAction = nil
	var execAction *ExecAction = newExecAction(egressProbeConfig.Exec, timeout)
	var grpcAction *GRPCAction = newGRPCAction(egressProbeConfig.GRPC, timeout)
	var httpGetAction *HTTPGetAction = newHTTPGetAction(egressProbeConfig.HTTPGet, timeout)
	var tcpSocketAction *TCPSocketAction = newTCPSocketAction(egressProbeConfig.TCPSocket, timeout)
	if execAction != nil {
		probeAction = execAction
	} else if grpcAction != nil {
		probeAction = grpcAction
	} else if httpGetAction != nil {
		probeAction = httpGetAction
	} else if tcpSocketAction != nil {
		probeAction = tcpSocketAction
	} else {
		logger.Error("no action for probe", "egressProbeConfig", egressProbeConfig)
		return nil
	}
	return &Probe{
		Name:               egressProbeConfig.Name,
		AttemptsMetric:     telemetry.NewAttemptsMetric(&egressProbeConfig.Metrics.Attempts, meter),
		ResponseTimeMetric: telemetry.NewResponseTimeMetric(&egressProbeConfig.Metrics.ResponseTime, meter),
		ProbeAction:        &probeAction,
	}
}
