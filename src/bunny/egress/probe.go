package egress

import (
	"bunny/config"
	"bunny/telemetry"
	"net"
	"syscall"
	"time"
)

type Probe struct {
	Name               string
	AttemptsMetric     *telemetry.CounterMetric
	ResponseTimeMetric *telemetry.ResponseTimeMetric
	SuccessesMetric    *telemetry.CounterMetric
	ProbeAction        *ProbeAction
}

type ProbeAction interface {
	act(probeName string, attemptsMetric *telemetry.CounterMetric, responseTimeMetric *telemetry.ResponseTimeMetric, successesMetric *telemetry.CounterMetric)
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
		AttemptsMetric:     telemetry.NewCounterMetric(&egressProbeConfig.Metrics.Attempts, meter),
		ResponseTimeMetric: telemetry.NewResponseTimeMetric(&egressProbeConfig.Metrics.ResponseTime, meter),
		SuccessesMetric:    telemetry.NewCounterMetric(&egressProbeConfig.Metrics.Successes, meter),
		ProbeAction:        &probeAction,
	}
}

// this is Kubernetes' implementation of creating a Dialer
// see: https://github.com/kubernetes/kubernetes/blob/master/pkg/probe/dialer_others.go#L33
func newDialer() *net.Dialer {
	return &net.Dialer{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				syscall.SetsockoptLinger(int(fd), syscall.SOL_SOCKET, syscall.SO_LINGER, &syscall.Linger{Onoff: 1, Linger: 1})
			})
		},
	}
}
