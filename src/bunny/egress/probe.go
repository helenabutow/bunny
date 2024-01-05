package egress

import (
	"bunny/config"
	"bunny/telemetry"
	"fmt"
	"net/http"
	"time"
)

// TODO-HIGH: implement performing the other probes
type Probe struct {
	Name               string
	AttemptsMetric     *telemetry.AttemptsMetric
	ResponseTimeMetric *telemetry.ResponseTimeMetric
	HTTPGetAction      *HTTPGetAction
}

type HTTPGetAction struct {
	httpProbeRequest *http.Request
	httpProbeClient  *http.Client
}

type ProbeAction interface {
	act(attemptsMetric *telemetry.AttemptsMetric, responseTimeMetric *telemetry.ResponseTimeMetric)
}

func newProbe(egressProbeConfig *config.EgressProbeConfig) *Probe {
	return &Probe{
		Name:               egressProbeConfig.Name,
		AttemptsMetric:     telemetry.NewAttemptsMetric(&egressProbeConfig.Metrics.Attempts, meter),
		ResponseTimeMetric: telemetry.NewResponseTimeMetric(&egressProbeConfig.Metrics.ResponseTime, meter),
		HTTPGetAction:      newHTTPGetAction(egressProbeConfig.HTTPGet),
	}
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

func (action HTTPGetAction) act(attemptsMetric *telemetry.AttemptsMetric, responseTimeMetric *telemetry.ResponseTimeMetric) {
	logger.Debug("performing http probe")
	// need to run this on a separate goroutine since the timeout could be greater than the period
	go func() {
		timerStart := telemetry.PreMeasurable(attemptsMetric, responseTimeMetric)
		response, err := action.httpProbeClient.Do(action.httpProbeRequest)
		telemetry.PostMeasurable(responseTimeMetric, timerStart)
		if err != nil || response.StatusCode != http.StatusOK {
			logger.Debug("probe failed")
		} else {
			logger.Debug("probe succeeded")
		}
	}()
}
