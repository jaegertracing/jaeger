package grpc

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/jaegertracing/jaeger/internal/bearertoken"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"
)

var (
	_ io.Closer = (*Factory)(nil)
)

type Factory struct {
	telset telemetry.Settings
	config Config

	traceReader *TraceReader
	traceWriter *TraceWriter

	readerConn *grpc.ClientConn
	writerConn *grpc.ClientConn
}

func NewFactory(
	cfg Config,
	telset telemetry.Settings,
) (*Factory, error) {
	f := &Factory{
		telset: telset,
		config: cfg,
	}
	f.telset.TracerProvider = otel.GetTracerProvider()

	readerTelset := getTelset(f.telset, f.telset.TracerProvider)
	writerTelset := getTelset(f.telset, noop.NewTracerProvider())
	newClientFn := func(telset component.TelemetrySettings, opts ...grpc.DialOption) (conn *grpc.ClientConn, err error) {
		clientOpts := make([]configgrpc.ToClientConnOption, 0)
		for _, opt := range opts {
			clientOpts = append(clientOpts, configgrpc.WithGrpcDialOption(opt))
		}
		return f.config.ToClientConn(context.Background(), f.telset.Host, telset, clientOpts...)
	}

	f.initializeClients(readerTelset, writerTelset, newClientFn)

	return f, nil
}

func (f *Factory) Close() error {
	var errs []error
	if f.readerConn != nil {
		errs = append(errs, f.readerConn.Close())
	}
	if f.writerConn != nil {
		errs = append(errs, f.writerConn.Close())
	}
	return errors.Join(errs...)
}

func getTelset(
	telset telemetry.Settings,
	tracerProvider trace.TracerProvider,
) component.TelemetrySettings {
	return component.TelemetrySettings{
		Logger:         telset.Logger,
		TracerProvider: tracerProvider,
		MeterProvider:  telset.MeterProvider,
	}
}

type newClientFn func(telset component.TelemetrySettings, opts ...grpc.DialOption) (*grpc.ClientConn, error)

func (f *Factory) initializeClients(
	readerTelset, writerTelset component.TelemetrySettings,
	newClient newClientFn,
) error {
	if f.config.Auth != nil {
		return errors.New("authenticator is not supported")
	}

	unaryInterceptors := []grpc.UnaryClientInterceptor{bearertoken.NewUnaryClientInterceptor()}
	streamInterceptors := []grpc.StreamClientInterceptor{bearertoken.NewStreamClientInterceptor()}

	if tenancyMgr := tenancy.NewManager(&f.config.Tenancy); tenancyMgr.Enabled {
		unaryInterceptors = append(unaryInterceptors, tenancy.NewClientUnaryInterceptor(tenancyMgr))
		streamInterceptors = append(streamInterceptors, tenancy.NewClientStreamInterceptor(tenancyMgr))
	}

	baseOpts := []grpc.DialOption{
		grpc.WithChainUnaryInterceptor(unaryInterceptors...),
		grpc.WithChainStreamInterceptor(streamInterceptors...),
	}

	createConn := func(telset component.TelemetrySettings) (*grpc.ClientConn, error) {
		opts := append(baseOpts, grpc.WithStatsHandler(
			otelgrpc.NewClientHandler(otelgrpc.WithTracerProvider(telset.TracerProvider)),
		))
		return newClient(telset, opts...)
	}

	readerConn, err := createConn(readerTelset)
	if err != nil {
		return fmt.Errorf("error creating reader client connection: %w", err)
	}
	writerConn, err := createConn(writerTelset)
	if err != nil {
		return fmt.Errorf("error creating writer client connection: %w", err)
	}

	f.readerConn, f.writerConn = readerConn, writerConn
	f.traceReader, f.traceWriter = NewTraceReader(readerConn), NewTraceWriter(writerConn)

	return nil
}
