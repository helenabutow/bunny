package egress

import (
	"bunny/config"
	"bunny/otel"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var logger *slog.Logger = nil
var ConfigUpdateChannel chan config.BunnyConfig = make(chan config.BunnyConfig, 1)
var OSSignalsChannel chan os.Signal = make(chan os.Signal, 1)
var ticker *time.Ticker = nil
var initialDelayTime time.Time = time.Now()
var egressConfig *config.EgressConfig = nil
var meter *metric.Meter = nil
var httpProbeRequest *http.Request = nil
var httpProbeClient *http.Client = nil
var extraAttributes *metric.MeasurementOption = nil
var probeAttempts int64 = 0
var probeAttemptsCounter *metric.Int64Counter = nil
var probeResponseTimeGauge *metric.Int64ObservableGauge = nil //lint:ignore U1000 we actually do use it (partly via the WithInt64Callback func)
var probeResponseTime *time.Duration = nil

func Init(sharedLogger *slog.Logger) {
	logger = sharedLogger
	logger.Info("Egress initializing")

	// yes, this looks silly
	// the goal here is to prevent the ticker from firing until config has been loaded
	ticker = time.NewTicker(1 * time.Hour)
	ticker.Stop()

	logger.Info("Egress is initialized")
}

func GoEgress(wg *sync.WaitGroup) {
	defer wg.Done()

	logger.Info("Egress is go!")

	for {
		logger.Debug("waiting for config or signal")
		select {
		case tickTime := <-ticker.C:
			if initialDelayTime.Before(time.Now()) {
				performProbe(&tickTime)
			}

		case bunnyConfig, ok := <-ConfigUpdateChannel:
			if !ok {
				continue
			}
			updateConfig(&bunnyConfig)

		case signal, ok := <-OSSignalsChannel:
			if !ok {
				logger.Error("could not process signal from signal channel")
			}
			logger.Info("received signal. Ending go routine.", "signal", signal)
			return
		}
	}
}

func updateConfig(bunnyConfig *config.BunnyConfig) {
	logger.Info("received config update")
	egressConfig = &bunnyConfig.EgressConfig
	meter = otel.Meter

	// process http probe config
	if egressConfig.HTTPGetActionConfig != nil {
		logger.Info("processing http probe config")
		var host string = *egressConfig.HTTPGetActionConfig.Host
		if host == "" {
			host = "localhost"
		}
		var url string = fmt.Sprintf("http://%s:%d/%s", host, egressConfig.HTTPGetActionConfig.Port, egressConfig.HTTPGetActionConfig.Path)
		logger.Debug("built url", "url", url)
		var err error
		httpProbeRequest, err = http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			logger.Error("could not build request for http probe", "err", err)
			return
		}
		for _, header := range egressConfig.HTTPGetActionConfig.HTTPHeaders {
			httpProbeRequest.Header.Add(header.Name, header.Value)
		}
		// this seems like the correct timeout based on https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts
		// (see the diagram in the "Client Timeouts" section)
		httpProbeClient = &http.Client{
			Timeout: time.Duration(egressConfig.TimeoutSeconds) * time.Second,
		}
	}

	// extra key/value pairs to include with each Prometheus metric
	attributesCopy := make([]attribute.KeyValue, len(egressConfig.EgressPrometheusConfig.ExtraEgressPrometheusLabels))
	for i, promLabelConfig := range egressConfig.EgressPrometheusConfig.ExtraEgressPrometheusLabels {
		attributesCopy[i] = attribute.Key(promLabelConfig.Name).String(promLabelConfig.Value)
	}
	newExtraAttributes := metric.WithAttributeSet(attribute.NewSet(attributesCopy...))
	extraAttributes = &newExtraAttributes

	// Prometheus metrics
	var metricName string = "egress_probe_attempts"
	if slices.Contains(egressConfig.EgressPrometheusConfig.MetricsEnabled, metricName) {
		newProbeAttemptsCounter, err := (*meter).Int64Counter(metricName)
		if err != nil {
			logger.Error("could not create probeAttemptsCounter", "err", err)
		}
		probeAttemptsCounter = &newProbeAttemptsCounter
	} else {
		probeAttemptsCounter = nil
	}
	metricName = "egress_probe_response_time"
	if slices.Contains(egressConfig.EgressPrometheusConfig.MetricsEnabled, metricName) {
		var unit = metric.WithUnit("ms")
		newProbeResponseTimeGauge, err := (*meter).Int64ObservableGauge(metricName, unit, metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			if probeResponseTime != nil {
				o.Observe(probeResponseTime.Milliseconds(), *extraAttributes)
				probeResponseTime = nil
			}
			return nil
		}))
		if err != nil {
			logger.Error("could not create probeResponseTimeGauge", "err", err)
		}
		probeResponseTimeGauge = &newProbeResponseTimeGauge

	} else {
		probeAttemptsCounter = nil
	}

	initialDelayTime = time.Now().Add(time.Duration(egressConfig.InitialDelaySeconds) * time.Second)
	logger.Debug("delay set", "initialDelayTime", initialDelayTime)

	ticker.Reset(time.Duration(egressConfig.PeriodSeconds) * time.Second)
	logger.Info("config update processing complete")
}

func performProbe(tickTime *time.Time) {
	logger.Debug("tick received", "tickTime", tickTime)

	if httpProbeClient != nil && httpProbeRequest != nil {
		incrementProbeAttempts()

		logger.Debug("performing http probe")
		// need to run this on a separate goroutine since the timeout could be greater than the period
		go func() {
			var startTime time.Time = time.Now()
			response, err := httpProbeClient.Do(httpProbeRequest)
			var endTime time.Time = time.Now()
			if err != nil || response.StatusCode != http.StatusOK {
				logger.Debug("probe failed")
			} else {
				logger.Debug("probe succeeded")
			}
			var newProbeResponseTime = endTime.Sub(startTime)
			probeResponseTime = &newProbeResponseTime
		}()
	}

	// TODO-HIGH: implement performing the other probes
}

func incrementProbeAttempts() {
	probeAttempts++
	logger.Debug("probeAttempts has increased", "probeAttempts", probeAttempts)
	if probeAttemptsCounter != nil {
		(*probeAttemptsCounter).Add(context.Background(), 1, *extraAttributes)
	}
}
