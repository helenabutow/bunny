package egress

import (
	"bunny/config"
	"bunny/telemetry"
	"context"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type HTTPGetAction struct {
	headers map[string][]string
	url     string
	client  *http.Client
	timeout time.Duration
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

func (action HTTPGetAction) act(probeName string, attemptsMetric *telemetry.CounterMetric, responseTimeMetric *telemetry.ResponseTimeMetric, successesMetric *telemetry.CounterMetric) {
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
			telemetry.PostMeasurable(successesMetric, responseTimeMetric, timerStart, false)
			message := ""
			if response == nil {
				message = "probe failed - no response"
			} else {
				message = fmt.Sprintf("probe failed - http response not ok: %v", response.StatusCode)
			}
			logger.Debug(message)
			span.SetStatus(codes.Error, message)
		} else {
			telemetry.PostMeasurable(successesMetric, responseTimeMetric, timerStart, true)
			message := "probe succeeded"
			logger.Debug(message)
			span.SetStatus(codes.Ok, message)
		}
	}()
}
