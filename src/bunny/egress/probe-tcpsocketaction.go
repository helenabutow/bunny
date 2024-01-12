package egress

import (
	"bunny/config"
	"bunny/telemetry"
	"context"
	"fmt"
	"net"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type TCPSocketAction struct {
	host        string
	port        int
	expectSteps []ExpectStep
	timeout     time.Duration
}

func newTCPSocketAction(tcpSocketActionConfig *config.TCPSocketActionConfig, timeout time.Duration) *TCPSocketAction {
	logger.Info("processing tcp socket probe config")
	if tcpSocketActionConfig == nil {
		return nil
	}

	var host = "localhost"
	if tcpSocketActionConfig.Host != nil {
		host = *tcpSocketActionConfig.Host
	}
	var expectSteps []ExpectStep = []ExpectStep{}
	if tcpSocketActionConfig.Expect != nil {
		for _, expectStepConfig := range *tcpSocketActionConfig.Expect {
			var expectStep ExpectStep = newExpectStep(&expectStepConfig)
			if expectStep != nil {
				expectSteps = append(expectSteps, expectStep)
			}
		}
	}

	// we don't create a client for tcp socket connections because it's just a net.Dial() call

	return &TCPSocketAction{
		host:        host,
		port:        tcpSocketActionConfig.Port,
		expectSteps: expectSteps,
		timeout:     timeout,
	}
}

func (action TCPSocketAction) act(probeName string, attemptsMetric *telemetry.AttemptsMetric, responseTimeMetric *telemetry.ResponseTimeMetric) {
	logger.Debug("performing tcp socket probe")
	// need to run this on a separate goroutine since the timeout could be greater than the period
	go func() {
		timeoutTime := time.Now().Add(action.timeout)
		timeoutContext, timeoutContextCancelFunc := context.WithDeadlineCause(context.Background(), timeoutTime, context.DeadlineExceeded)
		defer timeoutContextCancelFunc()

		// create the span
		_, span := (*tracer).Start(timeoutContext, "tcp-socket-probe")
		span.SetAttributes(attribute.KeyValue{
			Key:   "bunny-probe-name",
			Value: attribute.StringValue(probeName),
		})
		defer span.End()

		// connect to the tcp server
		host := "localhost"
		if action.host != "" {
			host = action.host
		}
		// TODO-HIGH: we need a "successfulAttempts" metric
		timerStart := telemetry.PreMeasurable(attemptsMetric, responseTimeMetric)
		var target = net.JoinHostPort(host, fmt.Sprintf("%v", action.port))
		timeoutDuration := time.Until(timeoutTime)
		tcpConnection, err := net.DialTimeout("tcp", target, timeoutDuration)
		if err != nil {
			message := "probe failed - could not connect to tcp server"
			telemetry.PostMeasurable(responseTimeMetric, timerStart, false)
			logger.Debug(message, "target", target, "err", err)
			span.SetStatus(codes.Error, message)
			return
		}
		defer tcpConnection.Close()
		tcpConnection.SetDeadline(timeoutTime)
		// check the expect steps
		expectSuccess := expect(&tcpConnection, action.expectSteps, &span)
		telemetry.PostMeasurable(responseTimeMetric, timerStart, expectSuccess)
		// thanks motivational code
		if !expectSuccess {
			message := "probe failed - expect steps failed"
			logger.Debug(message)
			span.SetStatus(codes.Error, message)
			return
		}
		message := "probe succeeded"
		logger.Debug(message)
		span.SetStatus(codes.Ok, message)
	}()
}
