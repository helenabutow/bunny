package egress

import (
	"bunny/config"
	"bunny/telemetry"
	"context"
	"fmt"
	"net"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
)

type GRPCAction struct {
	port    int
	service *string
	timeout time.Duration
}

func newGRPCAction(grpcActionConfig *config.GRPCActionConfig, timeout time.Duration) *GRPCAction {
	logger.Info("processing grpc probe config")
	if grpcActionConfig == nil {
		return nil
	}

	return &GRPCAction{
		port:    grpcActionConfig.Port,
		service: grpcActionConfig.Service,
		timeout: timeout,
	}
}

func (action GRPCAction) act(probeName string, attemptsMetric *telemetry.CounterMetric, responseTimeMetric *telemetry.ResponseTimeMetric, successesMetric *telemetry.CounterMetric) {
	logger.Debug("performing grpc probe")
	// need to run this on a separate goroutine since the timeout could be greater than the period
	go func() {
		timeoutTime := time.Now().Add(action.timeout)
		timeoutContext, timeoutContextCancelFunc := context.WithDeadlineCause(context.Background(), timeoutTime, context.DeadlineExceeded)
		defer timeoutContextCancelFunc()

		// create the span
		spanContext, span := (*tracer).Start(timeoutContext, "grpc-probe")
		span.SetAttributes(attribute.KeyValue{
			Key:   "bunny-probe-name",
			Value: attribute.StringValue(probeName),
		})
		defer span.End()

		// check the grpc server
		var err error
		var opts []grpc.CallOption = []grpc.CallOption{
			grpc.WaitForReady(false),
		}
		timerStart := telemetry.PreMeasurable(attemptsMetric, responseTimeMetric)
		// create the grpc client and connect to the server
		var target = net.JoinHostPort("localhost", fmt.Sprintf("%v", action.port))
		conn, err := grpc.DialContext(spanContext, target,
			grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
			grpc.WithBlock(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
				return newDialer().DialContext(ctx, "tcp", addr)
			}),
		)
		if err != nil {
			logger.Error("error while creating grpc client and connecting to server", "err", err)
			telemetry.PostMeasurable(successesMetric, responseTimeMetric, timerStart, false)
			return
		}
		defer conn.Close()
		client := healthgrpc.NewHealthClient(conn)
		// send the health check
		var response *healthgrpc.HealthCheckResponse
		if action.service == nil {
			logger.Debug("no service set - asking about general rpc server health")
			response, err = client.Check(spanContext, nil, opts...)
		} else {
			logger.Debug("service set - asking about health for service " + *action.service)
			healthCheckRequest := healthgrpc.HealthCheckRequest{
				Service: *action.service,
			}
			response, err = client.Check(spanContext, &healthCheckRequest, opts...)
		}
		message := ""
		if err != nil {
			message = "probe failed - could not check grpc server"
		} else if response == nil {
			message = "probe failed - response is nil"
		} else if response.GetStatus() != healthgrpc.HealthCheckResponse_SERVING {
			message = "probe failed - rpc server is not serving"
		}
		if message != "" {
			telemetry.PostMeasurable(successesMetric, responseTimeMetric, timerStart, false)
			logger.Debug(message, "response.GetStatus()", response.GetStatus())
			span.SetStatus(codes.Error, message)
			return
		}
		telemetry.PostMeasurable(successesMetric, responseTimeMetric, timerStart, true)
		message = "probe succeeded"
		logger.Debug(message)
		span.SetStatus(codes.Ok, message)
	}()
}
