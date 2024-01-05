package ingress

import (
	"bunny/config"
	"bunny/telemetry"
	"errors"
	"time"
)

type HealthEndpoint struct {
	Path  string
	Query *Query
}

type Query interface {
	exec() (bool, error)
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

func (q InstantQuery) exec() (bool, error) {
	instantTime := time.Now().Add(q.relativeInstantTime)
	return telemetry.InstantQuery(q.timeout, q.query, instantTime)
}

func (q RangeQuery) exec() (bool, error) {
	startTime := time.Now().Add(q.relativeStartTime)
	endTime := time.Now().Add(q.relativeEndTime)
	return telemetry.RangeQuery(q.timeout, q.query, startTime, endTime, q.interval)
}

func newHealthEndpoint(healthConfig *config.HealthConfig) (*HealthEndpoint, error) {
	if healthConfig.InstantQueryConfig == nil && healthConfig.RangeQueryConfig == nil {
		return nil, errors.New("neither instantQuery nor rangeQuery set for health endpoint at path " + healthConfig.Path)
	}
	if healthConfig.InstantQueryConfig != nil && healthConfig.RangeQueryConfig != nil {
		return nil, errors.New("both instantQuery and rangeQuery set for health endpoint at path " + healthConfig.Path)
	}
	if healthConfig.InstantQueryConfig != nil {
		query, err := newInstantQuery(healthConfig)
		return &HealthEndpoint{
			Path:  ensureLeadingSlash(healthConfig.Path),
			Query: &query,
		}, err
	} else {
		query, err := newRangeQuery(healthConfig)
		return &HealthEndpoint{
			Path:  ensureLeadingSlash(healthConfig.Path),
			Query: &query,
		}, err
	}
}

func newInstantQuery(healthConfig *config.HealthConfig) (Query, error) {
	timeout, err := time.ParseDuration(healthConfig.InstantQueryConfig.Timeout)
	if err != nil {
		logger.Error("error while parsing duration for timeout",
			"healthConfig.InstantQueryConfig", healthConfig.InstantQueryConfig)
		return nil, err
	}
	relativeInstantTime, err := time.ParseDuration(healthConfig.InstantQueryConfig.RelativeInstantTime)
	if err != nil {
		logger.Error("error while parsing duration for relativeInstantTime",
			"healthConfig.InstantQueryConfig", healthConfig.InstantQueryConfig)
		return nil, err
	}
	return InstantQuery{
		timeout:             timeout,
		relativeInstantTime: relativeInstantTime,
		query:               healthConfig.InstantQueryConfig.Query,
	}, err
}

func newRangeQuery(healthConfig *config.HealthConfig) (Query, error) {
	timeout, err := time.ParseDuration(healthConfig.RangeQueryConfig.Timeout)
	if err != nil {
		logger.Error("error while parsing duration for timeout",
			"healthConfig.RangeQueryConfig", healthConfig.RangeQueryConfig)
		return nil, err
	}
	relativeStartTime, err := time.ParseDuration(healthConfig.RangeQueryConfig.RelativeStartTime)
	if err != nil {
		logger.Error("error while parsing duration for relativeStartTime",
			"healthConfig.InstantQueryConfig", healthConfig.InstantQueryConfig)
		return nil, err
	}
	relativeEndTime, err := time.ParseDuration(healthConfig.RangeQueryConfig.RelativeEndTime)
	if err != nil {
		logger.Error("error while parsing duration for relativeEndTime",
			"healthConfig.InstantQueryConfig", healthConfig.InstantQueryConfig)
		return nil, err
	}
	interval, err := time.ParseDuration(healthConfig.RangeQueryConfig.Interval)
	if err != nil {
		logger.Error("error while parsing duration for interval",
			"healthConfig.InstantQueryConfig", healthConfig.InstantQueryConfig)
		return nil, err
	}
	return RangeQuery{
		timeout:           timeout,
		relativeStartTime: relativeStartTime,
		relativeEndTime:   relativeEndTime,
		interval:          interval,
		query:             healthConfig.RangeQueryConfig.Query,
	}, err
}
