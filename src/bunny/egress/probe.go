package egress

import (
	"bunny/config"
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// TODO-HIGH: implement performing the other probes
type Probe struct {
	Name               string
	AttemptsMetric     *AttemptsMetric
	ResponseTimeMetric *ResponseTimeMetric
	HTTPGetAction      *HTTPGetAction
}

type AttemptsMetric struct {
	Counter         *metric.Int64Counter
	ExtraAttributes metric.MeasurementOption
}

type ResponseTimeMetric struct {
	Gauge           *metric.Int64ObservableGauge
	ExtraAttributes metric.MeasurementOption
	Name            string
}

type HTTPGetAction struct {
	httpProbeRequest *http.Request
	httpProbeClient  *http.Client
}

type ProbeAction interface {
	act(attemptsMetric *AttemptsMetric, responseTimeMetric *ResponseTimeMetric)
}

func newProbe(egressProbeConfig *config.EgressProbeConfig) *Probe {
	return &Probe{
		Name:               egressProbeConfig.Name,
		AttemptsMetric:     newAttemptsMetric(&egressProbeConfig.EgressProbeMetricsConfig.Attempts),
		ResponseTimeMetric: newResponseTimeMetric(&egressProbeConfig.EgressProbeMetricsConfig.ResponseTime),
		HTTPGetAction:      newHTTPGetAction(egressProbeConfig.HTTPGetActionConfig),
	}
}

func newAttemptsMetric(egressMetricsConfig *config.EgressMetricsConfig) *AttemptsMetric {
	if !egressMetricsConfig.Enabled {
		return nil
	}

	var metricName string = egressMetricsConfig.Name
	newProbeAttemptsCounter, err := (*meter).Int64Counter(metricName)
	if err != nil {
		logger.Error("could not create newProbeAttemptsCounter", "err", err)
		return nil
	}

	return &AttemptsMetric{
		Counter:         &newProbeAttemptsCounter,
		ExtraAttributes: newAttributes(egressMetricsConfig),
	}
}

func newResponseTimeMetric(egressMetricsConfig *config.EgressMetricsConfig) *ResponseTimeMetric {
	if !egressMetricsConfig.Enabled {
		return nil
	}

	var metricName string = egressMetricsConfig.Name
	var unit = metric.WithUnit("ms")
	extraAttributes := newAttributes(egressMetricsConfig)
	newProbeResponseTimeGauge, err := (*meter).Int64ObservableGauge(metricName, unit, metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
		if metricName == "" {
			err := errors.New("unset metric name")
			logger.Error("unknown metric name within callback", "err", err)
			return err
		}
		probeResponseTime := probeResponseTimes[metricName]
		if probeResponseTime != nil {
			o.Observe(probeResponseTime.Milliseconds(), extraAttributes)
			probeResponseTimes[metricName] = nil
		}
		return nil
	}))
	if err != nil {
		logger.Error("could not create newProbeResponseTimeGauge", "err", err)
	}

	return &ResponseTimeMetric{
		Gauge:           &newProbeResponseTimeGauge,
		ExtraAttributes: extraAttributes,
		Name:            metricName,
	}
}

func newAttributes(egressMetricsConfig *config.EgressMetricsConfig) metric.MeasurementOption {
	attributesCopy := make([]attribute.KeyValue, len(egressMetricsConfig.EgressMetricsExtraLabels))
	for i, promLabelConfig := range egressMetricsConfig.EgressMetricsExtraLabels {
		attributesCopy[i] = attribute.Key(promLabelConfig.Name).String(promLabelConfig.Value)
	}
	return metric.WithAttributeSet(attribute.NewSet(attributesCopy...))
}

func newHTTPGetAction(httpGetActionConfig *config.HTTPGetActionConfig) *HTTPGetAction {
	logger.Info("processing http probe config")
	if httpGetActionConfig == nil {
		return nil
	}
	var host string = *httpGetActionConfig.Host
	if host == "" {
		host = "localhost"
	}
	var url string = fmt.Sprintf("http://%s:%d/%s", host, httpGetActionConfig.Port, httpGetActionConfig.Path)
	logger.Debug("built url", "url", url)
	newHTTPProbeRequest, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		logger.Error("could not build request for http probe", "err", err)
		return nil
	}
	for _, header := range httpGetActionConfig.HTTPHeaders {
		newHTTPProbeRequest.Header.Add(header.Name, header.Value)
	}
	// this seems like the correct timeout based on https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts
	// (see the diagram in the "Client Timeouts" section)
	newHTTPProbeClient := &http.Client{
		// TODO-MEDIUM: this reference to egressConfig seems gross and possibly buggy
		Timeout: time.Duration(egressConfig.TimeoutSeconds) * time.Second,
	}

	return &HTTPGetAction{
		httpProbeRequest: newHTTPProbeRequest,
		httpProbeClient:  newHTTPProbeClient,
	}
}

func (action HTTPGetAction) act(attemptsMetric *AttemptsMetric, responseTimeMetric *ResponseTimeMetric) {
	if attemptsMetric != nil {
		counter := attemptsMetric.Counter
		(*counter).Add(context.Background(), 1, attemptsMetric.ExtraAttributes)
	}

	logger.Debug("performing http probe")
	// need to run this on a separate goroutine since the timeout could be greater than the period
	go func() {
		var startTime time.Time = time.Now()
		response, err := action.httpProbeClient.Do(action.httpProbeRequest)
		var endTime time.Time = time.Now()
		if err != nil || response.StatusCode != http.StatusOK {
			logger.Debug("probe failed")
		} else {
			logger.Debug("probe succeeded")
		}
		var newProbeResponseTime time.Duration = endTime.Sub(startTime)
		if responseTimeMetric != nil {
			probeResponseTimes[responseTimeMetric.Name] = &newProbeResponseTime
		}
	}()
}
