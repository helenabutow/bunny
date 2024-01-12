package egress

import (
	"bunny/config"
	"bunny/telemetry"
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
)

type Probe struct {
	Name               string
	AttemptsMetric     *telemetry.AttemptsMetric
	ResponseTimeMetric *telemetry.ResponseTimeMetric
	ProbeAction        *ProbeAction
}

// TODO-MEDIUM: put the probe actions into separate files (to make things easier to find)
type ExecAction struct {
	command []string
	env     []string
	timeout time.Duration
}

type GRPCAction struct {
	port    int
	service *string
	timeout time.Duration
}

type HTTPGetAction struct {
	headers map[string][]string
	url     string
	client  *http.Client
	timeout time.Duration
}

type TCPSocketAction struct {
	host        string
	port        int
	expectSteps []ExpectStep
	timeout     time.Duration
}

type ProbeAction interface {
	act(probeName string, attemptsMetric *telemetry.AttemptsMetric, responseTimeMetric *telemetry.ResponseTimeMetric)
}

// TODO-MEDIUM: when we implement exec probes, consider adding support for env vars and baking this into the Docker image: https://github.com/equinix-labs/otel-cli#examples
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

func newExecAction(execActionConfig *config.ExecActionConfig, timeout time.Duration) *ExecAction {
	logger.Info("processing exec probe config")
	if execActionConfig == nil {
		return nil
	}

	// yes, this looks a bit strange but it's what exec.Cmd.Env needs
	envSlice := []string{}
	for _, envConfig := range execActionConfig.Env {
		envSliceItem := fmt.Sprintf("%v=%v", envConfig.Name, envConfig.Value)
		envSlice = append(envSlice, envSliceItem)
	}

	return &ExecAction{
		command: execActionConfig.Command,
		env:     envSlice,
		timeout: timeout,
	}
}

func newGRPCAction(grpcActionConfig *config.GRPCActionConfig, timeout time.Duration) *GRPCAction {
	logger.Info("processing grpc probe config")
	if grpcActionConfig == nil {
		return nil
	}

	return &GRPCAction{
		port:    grpcActionConfig.Port,
		service: grpcActionConfig.Service,
		timeout: timeout,
	}
}

func newHTTPGetAction(httpGetActionConfig *config.HTTPGetActionConfig, timeout time.Duration) *HTTPGetAction {
	logger.Info("processing http probe config")
	if httpGetActionConfig == nil {
		return nil
	}
	var host string = "localhost"
	if httpGetActionConfig.Host == nil && *httpGetActionConfig.Host != "" {
		host = *httpGetActionConfig.Host
	}
	var url string = fmt.Sprintf("http://%s:%d/%s", host, httpGetActionConfig.Port, httpGetActionConfig.Path)
	logger.Debug("built url", "url", url)

	// this seems like the correct timeout based on https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts
	// (see the diagram in the "Client Timeouts" section)
	client := &http.Client{
		Timeout:   timeout,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	// convert the headers into a map now so we don't have to do it later for each request
	var headers = map[string][]string{}
	for _, httpHeadersConfig := range httpGetActionConfig.HTTPHeaders {
		headers[httpHeadersConfig.Name] = httpHeadersConfig.Value
	}

	return &HTTPGetAction{
		headers: headers,
		url:     url,
		client:  client,
		timeout: timeout,
	}
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

func (action ExecAction) act(probeName string, attemptsMetric *telemetry.AttemptsMetric, responseTimeMetric *telemetry.ResponseTimeMetric) {
	logger.Debug("performing exec probe")
	// need to run this on a separate goroutine since the timeout could be greater than the period
	go func() {
		timeoutTime := time.Now().Add(action.timeout)
		timeoutContext, timeoutContextCancelFunc := context.WithDeadlineCause(context.Background(), timeoutTime, context.DeadlineExceeded)
		defer timeoutContextCancelFunc()

		// create the span
		spanContext, span := (*tracer).Start(timeoutContext, "exec-probe")
		span.SetAttributes(attribute.KeyValue{
			Key:   "bunny-probe-name",
			Value: attribute.StringValue(probeName),
		})
		defer span.End()

		// setup the environment variables
		// get the trace id (so that we can pass it to otel-cli)
		traceID := span.SpanContext().TraceID()
		tranceParent := "OTEL_CLI_FORCE_TRACE_ID=" + traceID.String()
		newEnvVars := append(action.env, tranceParent)

		// run the program
		cmd := exec.CommandContext(spanContext, action.command[0], action.command[1:]...)
		cmd.Env = newEnvVars
		timerStart := telemetry.PreMeasurable(attemptsMetric, responseTimeMetric)
		output, err := cmd.CombinedOutput()
		if err != nil {
			message := "probe failed - error while running command"
			telemetry.PostMeasurable(responseTimeMetric, timerStart, false)
			logger.Debug(message, "err", err, "output", string(output))
			span.SetStatus(codes.Error, message)
			return
		}
		telemetry.PostMeasurable(responseTimeMetric, timerStart, true)
		message := "probe succeeded"
		logger.Debug(message, "cmd.Path", cmd.Path, "cmd.Args", cmd.Args, "output", string(output))
		span.SetStatus(codes.Ok, message)
	}()
}

func (action GRPCAction) act(probeName string, attemptsMetric *telemetry.AttemptsMetric, responseTimeMetric *telemetry.ResponseTimeMetric) {
	logger.Debug("performing grpc probe")
	// need to run this on a separate goroutine since the timeout could be greater than the period
	go func() {
		timeoutTime := time.Now().Add(action.timeout)
		timeoutContext, timeoutContextCancelFunc := context.WithDeadlineCause(context.Background(), timeoutTime, context.DeadlineExceeded)
		defer timeoutContextCancelFunc()

		// create the span
		spanContext, span := (*tracer).Start(timeoutContext, "grpc-probe")
		span.SetAttributes(attribute.KeyValue{
			Key:   "bunny-probe-name",
			Value: attribute.StringValue(probeName),
		})
		defer span.End()

		// check the grpc server
		var err error
		var opts []grpc.CallOption = []grpc.CallOption{
			grpc.WaitForReady(false),
		}
		timerStart := telemetry.PreMeasurable(attemptsMetric, responseTimeMetric)
		// create the grpc client and connect to the server
		var target = net.JoinHostPort("localhost", fmt.Sprintf("%v", action.port))
		conn, err := grpc.Dial(target,
			grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
			grpc.WithBlock(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			logger.Error("error while creating grpc client and connecting to server", "err", err)
			telemetry.PostMeasurable(responseTimeMetric, timerStart, false)
			return
		}
		defer conn.Close()
		client := healthgrpc.NewHealthClient(conn)
		// send the health check
		var response *healthgrpc.HealthCheckResponse
		if action.service == nil {
			logger.Debug("no service set - asking about general rpc server health")
			response, err = client.Check(spanContext, nil, opts...)
		} else {
			logger.Debug("service set - asking about health for service " + *action.service)
			healthCheckRequest := healthgrpc.HealthCheckRequest{
				Service: *action.service,
			}
			response, err = client.Check(spanContext, &healthCheckRequest, opts...)
		}
		message := ""
		if err != nil {
			message = "probe failed - could not check grpc server"
		} else if response == nil {
			message = "probe failed - response is nil"
		} else if response.GetStatus() != healthgrpc.HealthCheckResponse_SERVING {
			message = "probe failed - rpc server is not serving"
		}
		if message != "" {
			telemetry.PostMeasurable(responseTimeMetric, timerStart, false)
			logger.Debug(message, "response.GetStatus()", response.GetStatus())
			span.SetStatus(codes.Error, message)
			return
		}
		telemetry.PostMeasurable(responseTimeMetric, timerStart, true)
		message = "probe succeeded"
		logger.Debug(message)
		span.SetStatus(codes.Ok, message)
	}()
}

func (action HTTPGetAction) act(probeName string, attemptsMetric *telemetry.AttemptsMetric, responseTimeMetric *telemetry.ResponseTimeMetric) {
	logger.Debug("performing http probe")
	// need to run this on a separate goroutine since the timeout could be greater than the period
	go func() {
		timeoutTime := time.Now().Add(action.timeout)
		timeoutContext, timeoutContextCancelFunc := context.WithDeadlineCause(context.Background(), timeoutTime, context.DeadlineExceeded)
		defer timeoutContextCancelFunc()

		// create the span
		spanContext, span := (*tracer).Start(timeoutContext, "http-probe")
		span.SetAttributes(attribute.KeyValue{
			Key:   "bunny-probe-name",
			Value: attribute.StringValue(probeName),
		})
		defer span.End()

		// create the http request
		// (we have to do it here instead of when creating the HTTPGetAction because we need the context for the span above)
		var url = action.url
		newHTTPProbeRequest, err := http.NewRequestWithContext(spanContext, http.MethodGet, url, nil)
		if err != nil {
			message := "probe failed - could not build request for http probe"
			logger.Debug(message)
			span.SetStatus(codes.Error, message)
			return
		}
		newHTTPProbeRequest.Close = true // disable keep alives to force creation of new connections on each request
		newHTTPProbeRequest.Header = action.headers

		timerStart := telemetry.PreMeasurable(attemptsMetric, responseTimeMetric)
		response, err := action.client.Do(newHTTPProbeRequest)
		if err != nil || response.StatusCode != http.StatusOK {
			telemetry.PostMeasurable(responseTimeMetric, timerStart, false)
			message := ""
			if response == nil {
				message = "probe failed - no response"
			} else {
				message = fmt.Sprintf("probe failed - http response not ok: %v", response.StatusCode)
			}
			logger.Debug(message)
			span.SetStatus(codes.Error, message)
		} else {
			telemetry.PostMeasurable(responseTimeMetric, timerStart, true)
			message := "probe succeeded"
			logger.Debug(message)
			span.SetStatus(codes.Ok, message)
		}
	}()
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
