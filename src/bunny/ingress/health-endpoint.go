package ingress

import (
	"bunny/config"
	"bunny/telemetry"
	"errors"
	"time"
)

type HealthEndpoint struct {
	Path               string
	Query              *Query
	AttemptsMetric     *telemetry.AttemptsMetric
	ResponseTimeMetric *telemetry.ResponseTimeMetric
}

type Query interface {
	exec(attemptsMetric *telemetry.AttemptsMetric, responseTimeMetric *telemetry.ResponseTimeMetric) (bool, error)
}

type InstantQuery struct {
	timeout             time.Duration
	relativeInstantTime time.Duration
	query               string
}

type RangeQuery struct {
	timeout           time.Duration
	relativeStartTime time.Duration
	relativeEndTime   time.Duration
	interval          time.Duration
	query             string
}

func (q InstantQuery) exec(attemptsMetric *telemetry.AttemptsMetric, responseTimeMetric *telemetry.ResponseTimeMetric) (bool, error) {
	instantTime := time.Now().Add(q.relativeInstantTime)
	timerStart := telemetry.PreMeasurable(attemptsMetric, responseTimeMetric)
	result, err := telemetry.InstantQuery(q.timeout, q.query, instantTime)
	telemetry.PostMeasurable(responseTimeMetric, timerStart)
	return result, err
}

func (q RangeQuery) exec(attemptsMetric *telemetry.AttemptsMetric, responseTimeMetric *telemetry.ResponseTimeMetric) (bool, error) {
	startTime := time.Now().Add(q.relativeStartTime)
	endTime := time.Now().Add(q.relativeEndTime)
	timerStart := telemetry.PreMeasurable(attemptsMetric, responseTimeMetric)
	result, err := telemetry.RangeQuery(q.timeout, q.query, startTime, endTime, q.interval)
	telemetry.PostMeasurable(responseTimeMetric, timerStart)
	return result, err
}

func newHealthEndpoint(healthConfig *config.HealthConfig) (*HealthEndpoint, error) {
	if healthConfig.InstantQuery == nil && healthConfig.RangeQuery == nil {
		return nil, errors.New("neither instantQuery nor rangeQuery set for health endpoint at path " + healthConfig.Path)
	}
	if healthConfig.InstantQuery != nil && healthConfig.RangeQuery != nil {
		return nil, errors.New("both instantQuery and rangeQuery set for health endpoint at path " + healthConfig.Path)
	}

	var err error
	var query Query
	if healthConfig.InstantQuery != nil {
		query, err = newInstantQuery(healthConfig)
	} else {
		query, err = newRangeQuery(healthConfig)
	}
	return &HealthEndpoint{
		Path:               ensureLeadingSlash(healthConfig.Path),
		Query:              &query,
		AttemptsMetric:     telemetry.NewAttemptsMetric(&healthConfig.Metrics.Attempts, meter),
		ResponseTimeMetric: telemetry.NewResponseTimeMetric(&healthConfig.Metrics.ResponseTime, meter),
	}, err
}

func newInstantQuery(healthConfig *config.HealthConfig) (Query, error) {
	timeout, err := time.ParseDuration(healthConfig.InstantQuery.Timeout)
	if err != nil {
		logger.Error("error while parsing duration for timeout",
			"healthConfig.InstantQueryConfig", healthConfig.InstantQuery)
		return nil, err
	}
	relativeInstantTime, err := time.ParseDuration(healthConfig.InstantQuery.RelativeInstantTime)
	if err != nil {
		logger.Error("error while parsing duration for relativeInstantTime",
			"healthConfig.InstantQueryConfig", healthConfig.InstantQuery)
		return nil, err
	}
	return InstantQuery{
		timeout:             timeout,
		relativeInstantTime: relativeInstantTime,
		query:               healthConfig.InstantQuery.Query,
	}, err
}

func newRangeQuery(healthConfig *config.HealthConfig) (Query, error) {
	timeout, err := time.ParseDuration(healthConfig.RangeQuery.Timeout)
	if err != nil {
		logger.Error("error while parsing duration for timeout",
			"healthConfig.RangeQueryConfig", healthConfig.RangeQuery)
		return nil, err
	}
	relativeStartTime, err := time.ParseDuration(healthConfig.RangeQuery.RelativeStartTime)
	if err != nil {
		logger.Error("error while parsing duration for relativeStartTime",
			"healthConfig.InstantQueryConfig", healthConfig.InstantQuery)
		return nil, err
	}
	relativeEndTime, err := time.ParseDuration(healthConfig.RangeQuery.RelativeEndTime)
	if err != nil {
		logger.Error("error while parsing duration for relativeEndTime",
			"healthConfig.InstantQueryConfig", healthConfig.InstantQuery)
		return nil, err
	}
	interval, err := time.ParseDuration(healthConfig.RangeQuery.Interval)
	if err != nil {
		logger.Error("error while parsing duration for interval",
			"healthConfig.InstantQueryConfig", healthConfig.InstantQuery)
		return nil, err
	}
	return RangeQuery{
		timeout:           timeout,
		relativeStartTime: relativeStartTime,
		relativeEndTime:   relativeEndTime,
		interval:          interval,
		query:             healthConfig.RangeQuery.Query,
	}, err
}
