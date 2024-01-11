package main

import (
	"bufio"
	"bunny/logging"
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdk_trace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
)

var logger *slog.Logger = nil
var httpServer *http.Server = nil
var tracer *trace.Tracer = nil

func startGRPCEndpoint() {
	logger.Info("starting gRPC server")
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", 3936))
	if err != nil {
		logger.Error("could not start gRPC endpoint", err)
		return
	}
	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	)
	healthServer := health.NewServer()
	healthgrpc.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("health", healthgrpc.HealthCheckResponse_SERVING)
	go func() {
		err := grpcServer.Serve(listener)
		logger.Error("Error while serving gRPC", "err", err)
	}()
	logger.Info("done starting gRPC server")
}

func main() {
	logger = logging.ConfigureLogger("main")
	logger.Info("begin")

	// setup otel
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	exporter, err := otlptracehttp.New(context.Background())
	if err != nil {
		logger.Error("error while creating otlptracehttp exporter", "err", err)
		return
	}
	var traceProviderOptions []sdk_trace.TracerProviderOption = []sdk_trace.TracerProviderOption{}
	traceProviderOptions = append(traceProviderOptions, sdk_trace.WithBatcher(exporter))
	var serviceNameAttribute = attribute.String("service.name", "bee")
	var serviceNameResource = resource.NewWithAttributes("", serviceNameAttribute)
	traceProviderOptions = append(traceProviderOptions, sdk_trace.WithResource(serviceNameResource))
	var traceProvider = sdk_trace.NewTracerProvider(traceProviderOptions...)
	otel.SetTracerProvider(traceProvider)
	newTracer := otel.GetTracerProvider().Tracer("bunny/egress")
	tracer = &newTracer

	startGRPCEndpoint()
	startHTTPEndpoint()
	startTCPEndpoint()

	// wait for OS signal
	var osSignalsChannel chan os.Signal = make(chan os.Signal, 1)
	signal.Notify(osSignalsChannel, syscall.SIGINT, syscall.SIGTERM)
	signal, ok := <-osSignalsChannel
	if !ok {
		logger.Error("could not process signal from signal channel")
	}
	logger.Info("received signal", "signal", signal)

	logger.Info("end")
}

func startTCPEndpoint() {
	logger.Info("starting TCP server")

	listener, err := net.Listen("tcp", ":5248")
	if err != nil {
		logger.Error("Error starting TCP listener", "err", err)
	}
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				logger.Error("Error accepting TCP connection", "err", err)
			} else {
				go handleTCPConnection(&connection)
			}
		}
	}()

	logger.Info("done starting TCP server")
}

func handleTCPConnection(connection *net.Conn) {
	logger.Debug("handling tcp connection")

	_, span := (*tracer).Start(context.Background(), "handleTCPConnection")
	defer span.End()

	defer (*connection).Close()
	(*connection).SetDeadline(time.Now().Add(time.Duration(10) * time.Second))
	reader := bufio.NewReader(*connection)
	writer := bufio.NewWriter(*connection)

	if tcpRead(reader, "hello") &&
		tcpWrite(writer, "yellow") &&
		tcpRead(reader, "how're ya now?") &&
		tcpWrite(writer, "can't complain") {
		message := "successful tcp conversation"
		logger.Debug(message)
		span.SetStatus(codes.Ok, message)
	} else {
		message := "failed tcp conversation"
		logger.Debug(message)
		span.SetStatus(codes.Error, message)
	}

	logger.Debug("done handling tcp connection")
}

func tcpRead(reader *bufio.Reader, words string) bool {
	logger.Debug("expecting to read", "words", words)
	wordsSent, err := reader.ReadString('\n')
	if err != nil {
		logger.Debug("could not read - error", "err", err)
		return false
	}
	// since the delimiter is included in the response, we include it in the check
	if (words + "\n") != wordsSent {
		logger.Debug("could not read - incorrect string", "wordsSent", wordsSent)
		return false
	}
	logger.Debug("read successful")
	return true
}

func tcpWrite(writer *bufio.Writer, words string) bool {
	logger.Debug("expecting to write", "words", words)
	fullText := words + "\n"
	for i := 0; i < len(fullText); {
		remainingWords := fullText[i:]
		bytesWritten, err := fmt.Fprint(writer, remainingWords)
		if bytesWritten == 0 {
			logger.Debug("could not write - no bytes written")
			return false
		}
		if err != nil {
			logger.Debug("could not write - write error", "err", err)
			return false
		}
		i += bytesWritten
	}
	err := writer.Flush()
	if err != nil {
		logger.Debug("could not write - flush error", "err", err)
		return false
	}
	logger.Debug("write successful")
	return true
}

func startHTTPEndpoint() {
	logger.Info("starting HTTP server")
	mux := http.NewServeMux()
	handleFunc := func(pattern string, handlerFunc func(http.ResponseWriter, *http.Request)) {
		// Configure the "http.route" for the HTTP instrumentation.
		handler := otelhttp.WithRouteTag(pattern, http.HandlerFunc(handlerFunc))
		mux.Handle(pattern, handler)
	}
	handleFunc("/healthz", healthEndpoint)
	otelHandler := otelhttp.NewHandler(mux, "/")

	httpServer = &http.Server{
		Addr:              ":" + fmt.Sprintf("%d", 2624),
		ReadTimeout:       0,
		ReadHeaderTimeout: 0,
		WriteTimeout:      0,
		IdleTimeout:       0,
		MaxHeaderBytes:    0,
		Handler:           otelHandler,
	}

	go func() {
		err := httpServer.ListenAndServe()
		if err != http.ErrServerClosed {
			logger.Error("Error starting or closing http listener", "err", err)
		}
	}()
	logger.Info("done starting HTTP server")
}

func healthEndpoint(response http.ResponseWriter, request *http.Request) {
	logger.Debug("headers", "request.Header", request.Header)

	childContext, span := (*tracer).Start(request.Context(), "healthEndpoint")
	defer span.End()

	if checkHealth(childContext) {
		response.WriteHeader(http.StatusOK)
		fmt.Fprintln(response, "healthy")
	} else {
		response.WriteHeader(http.StatusRequestTimeout)
		fmt.Fprintln(response, "unhealthy")
	}
}

func checkHealth(ctx context.Context) bool {
	// health is based on a a sine wave
	const period int64 = 100
	const maxDelay float64 = 1.0
	var percent = float64(time.Now().Unix()%period) / float64(period)
	var x float64 = 2.0 * (float64(math.Pi)) * percent
	var y = (math.Sin(x) + 1.0) / 2.0
	var delay = time.Duration(y * maxDelay * float64(time.Second))
	logger.Debug("health calc",
		"percent", percent,
		"x", x,
		"y", y,
		"delay", delay,
	)
	var timeToStop = time.Now().Add(delay)
	timeWaster(ctx, timeToStop, "timeWaster")
	if y > 0.0 {
		logger.Debug("healthy")
		return true
	} else {
		logger.Debug("unhealthy")
		return false
	}
}

func timeWaster(parentContext context.Context, timeToStop time.Time, childName string) {
	childContext, span := (*tracer).Start(parentContext, childName)
	defer span.End()

	logger.Debug("timeWaster start", "childName", childName, "timeToStop", timeToStop)

	var timeAvailableMilliseconds = time.Until(timeToStop).Milliseconds()
	if timeAvailableMilliseconds <= 10 {
		return
	}
	if len(childName) > 20 {
		return
	}
	// waste time
	// this isn't a balanced tree but it's fine for something to test against
	var delays []int64 = []int64{0, 0, 0}
	delays[0] = rand.Int63n(timeAvailableMilliseconds) * 40 / 100
	delays[1] = rand.Int63n(timeAvailableMilliseconds) * 40 / 100
	delays[2] = timeAvailableMilliseconds - delays[0] - delays[1]
	rand.Shuffle(len(delays), func(i, j int) { delays[i], delays[j] = delays[j], delays[i] })
	var newChildName string = makeChildName(childName, 0)
	timeWaster(childContext, time.Now().Add(time.Duration(delays[0])*time.Millisecond), newChildName)
	time.Sleep(time.Duration(delays[1]) * time.Millisecond)
	newChildName = makeChildName(childName, 1)
	timeWaster(childContext, time.Now().Add(time.Duration(delays[2])*time.Millisecond), newChildName)

	// the safety sleep to ensure that we waste time till it's time to stop
	time.Sleep(time.Until(timeToStop))
}

func makeChildName(parentName string, i int) string {
	if parentName == "timeWaster" {
		return fmt.Sprintf("%v-%v", parentName, i)
	} else {
		return fmt.Sprintf("%v%v", parentName, i)
	}
}
