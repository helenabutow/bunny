package egress

import (
	"bunny/config"
	"bunny/telemetry"
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	client_golang_prometheus "github.com/prometheus/client_golang/prometheus"
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
	OtelCounter         *metric.Int64Counter
	OtelExtraAttributes metric.MeasurementOption
	PromCounter         client_golang_prometheus.Counter
}

type ResponseTimeMetric struct {
	OtelGauge           *metric.Int64ObservableGauge
	OtelExtraAttributes metric.MeasurementOption
	OtelMetricName      string
	PromGauge           client_golang_prometheus.Gauge
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
	newProbeAttemptsCounter, err := (*meter).Int64Counter("otel_" + metricName)
	if err != nil {
		logger.Error("could not create newProbeAttemptsCounter", "err", err)
		return nil
	}

	var opts client_golang_prometheus.CounterOpts = client_golang_prometheus.CounterOpts{
		Name:        "prom_" + metricName,
		ConstLabels: newLabels(egressMetricsConfig),
	}
	var newPromCounter = client_golang_prometheus.NewCounter(opts)
	telemetry.PromRegistry.Unregister(newPromCounter)
	telemetry.PromRegistry.MustRegister(newPromCounter)

	return &AttemptsMetric{
		OtelCounter:         &newProbeAttemptsCounter,
		OtelExtraAttributes: newAttributes(egressMetricsConfig),
		PromCounter:         newPromCounter,
	}
}

func newResponseTimeMetric(egressMetricsConfig *config.EgressMetricsConfig) *ResponseTimeMetric {
	if !egressMetricsConfig.Enabled {
		return nil
	}

	var metricName string = egressMetricsConfig.Name
	var unit = metric.WithUnit("ms")
	extraAttributes := newAttributes(egressMetricsConfig)
	newProbeResponseTimeGauge, err := (*meter).Int64ObservableGauge("otel_"+metricName, unit, metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
		if metricName == "" {
			err := errors.New("unset metric name")
			logger.Error("unknown metric name within callback", "err", err)
			return err
		}
		probeResponseTimesMutex.Lock()
		defer probeResponseTimesMutex.Unlock()
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

	var opts client_golang_prometheus.GaugeOpts = client_golang_prometheus.GaugeOpts{
		Name:        "prom_" + metricName,
		ConstLabels: newLabels(egressMetricsConfig),
	}
	var newPromGauge = client_golang_prometheus.NewGauge(opts)
	telemetry.PromRegistry.Unregister(newPromGauge)
	telemetry.PromRegistry.MustRegister(newPromGauge)

	return &ResponseTimeMetric{
		OtelGauge:           &newProbeResponseTimeGauge,
		OtelExtraAttributes: extraAttributes,
		PromGauge:           newPromGauge,
		OtelMetricName:      metricName,
	}
}

func newAttributes(egressMetricsConfig *config.EgressMetricsConfig) metric.MeasurementOption {
	attributesCopy := make([]attribute.KeyValue, len(egressMetricsConfig.EgressMetricsExtraLabels))
	for i, promLabelConfig := range egressMetricsConfig.EgressMetricsExtraLabels {
		attributesCopy[i] = attribute.Key(promLabelConfig.Name).String(promLabelConfig.Value)
	}
	return metric.WithAttributeSet(attribute.NewSet(attributesCopy...))
}

func newLabels(egressMetricsConfig *config.EgressMetricsConfig) client_golang_prometheus.Labels {
	var m map[string]string = map[string]string{}
	for _, egressMetricsExtraLabel := range egressMetricsConfig.EgressMetricsExtraLabels {
		m[egressMetricsExtraLabel.Name] = egressMetricsExtraLabel.Value
	}
	logger.Debug("new labels", "m", m)
	return m
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
		Timeout: time.Duration(egressConfig.TimeoutMilliseconds) * time.Millisecond,
	}

	return &HTTPGetAction{
		httpProbeRequest: newHTTPProbeRequest,
		httpProbeClient:  newHTTPProbeClient,
	}
}

func (action HTTPGetAction) act(attemptsMetric *AttemptsMetric, responseTimeMetric *ResponseTimeMetric) {
	if attemptsMetric != nil {
		counter := attemptsMetric.OtelCounter
		(*counter).Add(context.Background(), 1, attemptsMetric.OtelExtraAttributes)
		attemptsMetric.PromCounter.Inc()
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
			probeResponseTimesMutex.Lock()
			defer probeResponseTimesMutex.Unlock()
			probeResponseTimes[responseTimeMetric.OtelMetricName] = &newProbeResponseTime

			// unlike with OpenTelemetry, we can append the value to the Prometheus TSDB immediately
			responseTimeMetric.PromGauge.Set(float64(newProbeResponseTime.Milliseconds()))
		}
	}()
}
