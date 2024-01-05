package telemetry

import (
	"bunny/common"
	"bunny/config"
	"context"
	"errors"
	"time"

	client_golang_prometheus "github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/metric"
)

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

func PreMeasurable(attemptsMetric *AttemptsMetric, responseTimeMetric *ResponseTimeMetric) *time.Time {
	if attemptsMetric != nil {
		counter := attemptsMetric.OtelCounter
		(*counter).Add(context.Background(), 1, attemptsMetric.OtelExtraAttributes)
		attemptsMetric.PromCounter.Inc()
	}
	if responseTimeMetric != nil {
		timerStart := time.Now()
		return &timerStart
	}
	return nil
}

func PostMeasurable(responseTimeMetric *ResponseTimeMetric, timerStart *time.Time) {
	if responseTimeMetric != nil {
		timerEnd := time.Now()
		responseTime := timerEnd.Sub(*timerStart)

		common.ResponseTimesMutex.Lock()
		defer common.ResponseTimesMutex.Unlock()
		common.ResponseTimes[responseTimeMetric.OtelMetricName] = &responseTime

		// unlike with OpenTelemetry, we can append the value to the Prometheus TSDB immediately
		responseTimeMetric.PromGauge.Set(float64(responseTime.Milliseconds()))
	}
}

func NewAttemptsMetric(metricsConfig *config.MetricsConfig, meter *metric.Meter) *AttemptsMetric {
	if !metricsConfig.Enabled {
		return nil
	}

	var metricName string = metricsConfig.Name
	newAttemptsCounter, err := (*meter).Int64Counter("otel_" + metricName)
	if err != nil {
		logger.Error("could not create newAttemptsCounter", "err", err)
		return nil
	}

	var opts client_golang_prometheus.CounterOpts = client_golang_prometheus.CounterOpts{
		Name:        "prom_" + metricName,
		ConstLabels: NewLabels(metricsConfig.ExtraLabels),
	}
	var newPromCounter = client_golang_prometheus.NewCounter(opts)
	PromRegistry.Unregister(newPromCounter)
	PromRegistry.MustRegister(newPromCounter)

	return &AttemptsMetric{
		OtelCounter:         &newAttemptsCounter,
		OtelExtraAttributes: NewAttributes(metricsConfig.ExtraLabels),
		PromCounter:         newPromCounter,
	}
}

func NewResponseTimeMetric(metricsConfig *config.MetricsConfig, meter *metric.Meter) *ResponseTimeMetric {
	if !metricsConfig.Enabled {
		return nil
	}

	var metricName string = metricsConfig.Name
	var unit = metric.WithUnit("ms")
	extraAttributes := NewAttributes(metricsConfig.ExtraLabels)
	newResponseTimeGauge, err := (*meter).Int64ObservableGauge("otel_"+metricName, unit, metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
		if metricName == "" {
			err := errors.New("unset metric name")
			logger.Error("unknown metric name within callback", "err", err)
			return err
		}
		common.ResponseTimesMutex.Lock()
		defer common.ResponseTimesMutex.Unlock()
		responseTime := common.ResponseTimes[metricName]
		if responseTime != nil {
			o.Observe(responseTime.Milliseconds(), extraAttributes)
			common.ResponseTimes[metricName] = nil
		}
		return nil
	}))
	if err != nil {
		logger.Error("could not create newResponseTimeGauge", "err", err)
	}

	var opts client_golang_prometheus.GaugeOpts = client_golang_prometheus.GaugeOpts{
		Name:        "prom_" + metricName,
		ConstLabels: NewLabels(metricsConfig.ExtraLabels),
	}
	var newPromGauge = client_golang_prometheus.NewGauge(opts)
	PromRegistry.Unregister(newPromGauge)
	PromRegistry.MustRegister(newPromGauge)

	return &ResponseTimeMetric{
		OtelGauge:           &newResponseTimeGauge,
		OtelExtraAttributes: extraAttributes,
		PromGauge:           newPromGauge,
		OtelMetricName:      metricName,
	}
}
