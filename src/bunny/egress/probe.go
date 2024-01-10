package egress

import (
	"bunny/config"
	"bunny/telemetry"
	"context"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
)

// TODO-HIGH: implement performing the other probes
type Probe struct {
	Name               string
	AttemptsMetric     *telemetry.AttemptsMetric
	ResponseTimeMetric *telemetry.ResponseTimeMetric
	GRPCAction         *GRPCAction
	HTTPGetAction      *HTTPGetAction
}

type GRPCAction struct {
	port    int
	service *string
	client  *healthgrpc.HealthClient
}

type HTTPGetAction struct {
	headers map[string][]string
	url     string
	client  *http.Client
}

type ProbeAction interface {
	act(attemptsMetric *telemetry.AttemptsMetric, responseTimeMetric *telemetry.ResponseTimeMetric)
}

func newProbe(egressProbeConfig *config.EgressProbeConfig, timeout time.Duration) *Probe {
	return &Probe{
		Name:               egressProbeConfig.Name,
		AttemptsMetric:     telemetry.NewAttemptsMetric(&egressProbeConfig.Metrics.Attempts, meter),
		ResponseTimeMetric: telemetry.NewResponseTimeMetric(&egressProbeConfig.Metrics.ResponseTime, meter),
		GRPCAction:         newGRPCAction(egressProbeConfig.GRPC, timeout),
		HTTPGetAction:      newHTTPGetAction(egressProbeConfig.HTTPGet, timeout),
	}
}

func newGRPCAction(grpcActionConfig *config.GRPCActionConfig, timeout time.Duration) *GRPCAction {
	logger.Info("processing grpc probe config")
	if grpcActionConfig == nil {
		return nil
	}

	// create the grpc client
	var target = fmt.Sprintf("localhost:%v", grpcActionConfig.Port)
	conn, err := grpc.Dial(target,
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithBlock(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		logger.Error("error while creating grpc client", "err", err)
		return nil
	}
	client := healthgrpc.NewHealthClient(conn)

	return &GRPCAction{
		port:    grpcActionConfig.Port,
		service: grpcActionConfig.Service,
		client:  &client,
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
	}
}

func (action GRPCAction) act(attemptsMetric *telemetry.AttemptsMetric, responseTimeMetric *telemetry.ResponseTimeMetric) {
	logger.Debug("performing grpc probe")
	// need to run this on a separate goroutine since the timeout could be greater than the period
	go func() {
		// create the span
		// TODO-HIGH: we need to include the probe name (from bunny.yaml) somehow (maybe as an attribute of some sort?)
		// do this for the http probe as well
		spanContext, span := (*tracer).Start(context.Background(), "grpc-probe")
		defer span.End()

		// check the grpc server
		var opts []grpc.CallOption = []grpc.CallOption{
			grpc.WaitForReady(false),
		}
		timerStart := telemetry.PreMeasurable(attemptsMetric, responseTimeMetric)
		var response *healthgrpc.HealthCheckResponse
		var err error
		if action.service == nil {
			logger.Debug("no service set - asking about general rpc server health")
			response, err = (*action.client).Check(spanContext, nil, opts...)
		} else {
			logger.Debug("service set - asking about health for service " + *action.service)
			healthCheckRequest := healthgrpc.HealthCheckRequest{
				Service: *action.service,
			}
			response, err = (*action.client).Check(spanContext, &healthCheckRequest, opts...)
		}
		telemetry.PostMeasurable(responseTimeMetric, timerStart)
		message := ""
		if err != nil {
			message = "probe failed - could not check grpc server"
		}
		if response == nil {
			message = "probe failed - response is nil"
		}
		if response.GetStatus() != healthgrpc.HealthCheckResponse_SERVING {
			message = "probe failed - rpc server is not serving"
		}
		if message != "" {
			logger.Debug(message, "response.GetStatus()", response.GetStatus())
			span.SetStatus(codes.Error, message)
			return
		}
		message = "probe succeeded"
		logger.Debug(message)
		span.SetStatus(codes.Ok, message)
	}()
}

func (action HTTPGetAction) act(attemptsMetric *telemetry.AttemptsMetric, responseTimeMetric *telemetry.ResponseTimeMetric) {
	logger.Debug("performing http probe")
	// need to run this on a separate goroutine since the timeout could be greater than the period
	go func() {
		// create the span
		spanContext, span := (*tracer).Start(context.Background(), "http-probe")
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
		newHTTPProbeRequest.Header = action.headers

		timerStart := telemetry.PreMeasurable(attemptsMetric, responseTimeMetric)
		response, err := action.client.Do(newHTTPProbeRequest)
		telemetry.PostMeasurable(responseTimeMetric, timerStart)
		if err != nil || response.StatusCode != http.StatusOK {
			message := ""
			if response == nil {
				message = "probe failed - no response"
			} else {
				message = fmt.Sprintf("probe failed - http response not ok: %v", response.StatusCode)
			}
			logger.Debug(message)
			span.SetStatus(codes.Error, message)
		} else {
			message := "probe succeeded"
			logger.Debug(message)
			span.SetStatus(codes.Ok, message)
		}
	}()
}
