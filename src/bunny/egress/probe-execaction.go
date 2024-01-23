package egress

import (
	"bunny/config"
	"bunny/telemetry"
	"context"
	"fmt"
	"os/exec"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type ExecAction struct {
	command []string
	env     []string
	timeout time.Duration
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

func (action ExecAction) act(probeName string, attemptsMetric *telemetry.CounterMetric, responseTimeMetric *telemetry.ResponseTimeMetric, successesMetric *telemetry.CounterMetric) {
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
		// TODO-MEDIUM: does this fail if no args are provided to the command?
		cmd := exec.CommandContext(spanContext, action.command[0], action.command[1:]...)
		cmd.Env = newEnvVars
		timerStart := telemetry.PreMeasurable(attemptsMetric, responseTimeMetric)
		output, err := cmd.CombinedOutput()
		if err != nil {
			message := "probe failed - error while running command"
			telemetry.PostMeasurable(successesMetric, responseTimeMetric, timerStart, false)
			logger.Debug(message, "err", err, "output", string(output))
			span.SetStatus(codes.Error, message)
			return
		}
		telemetry.PostMeasurable(successesMetric, responseTimeMetric, timerStart, true)
		message := "probe succeeded"
		// TODO-LOW: limit the amount of stdout and stderr to limit memory usage from incredibly noisy exec programs/scripts
		// see how Kubernetes does this for their version of the exec probe action
		logger.Debug(message, "cmd.Path", cmd.Path, "cmd.Args", cmd.Args, "output", string(output))
		span.SetStatus(codes.Ok, message)
	}()
}
