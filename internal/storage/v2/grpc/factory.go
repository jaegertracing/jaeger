// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"errors"
	"fmt"
	"io"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/internal/auth/bearertoken"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

var (
	_ io.Closer          = (*Factory)(nil)
	_ tracestore.Factory = (*Factory)(nil)
	_ depstore.Factory   = (*Factory)(nil)
)

type Factory struct {
	telset telemetry.Settings
	config Config
	// readerConn is the gRPC connection used for reading data from the remote storage backend.
	// It is safe for this connection to have instrumentation enabled without
	// the risk of recursively generating traces.
	readerConn *grpc.ClientConn
	// writerConn is the gRPC connection used for writing data to the remote storage backend.
	// This connection should not have instrumentation enabled to avoid recursively generating traces.
	writerConn *grpc.ClientConn
}

// NewFactory initializes a new gRPC (remote) storage backend.
func NewFactory(
	ctx context.Context,
	cfg Config,
	telset telemetry.Settings,
) (*Factory, error) {
	f := &Factory{
		telset: telset,
		config: cfg,
	}

	var writerConfig configgrpc.ClientConfig
	if cfg.Writer.Endpoint != "" {
		writerConfig = cfg.Writer
	} else {
		writerConfig = cfg.ClientConfig
	}

	readerTelset := getTelset(f.telset, f.telset.TracerProvider)
	writerTelset := getTelset(f.telset, noop.NewTracerProvider())
	newClientFn := func(telset component.TelemetrySettings, gcs *configgrpc.ClientConfig, opts ...grpc.DialOption) (conn *grpc.ClientConn, err error) {
		clientOpts := make([]configgrpc.ToClientConnOption, 0)
		for _, opt := range opts {
			clientOpts = append(clientOpts, configgrpc.WithGrpcDialOption(opt))
		}
		return gcs.ToClientConn(ctx, f.telset.Host, telset, clientOpts...)
	}

	if err := f.initializeConnections(readerTelset, writerTelset, &cfg.ClientConfig, &writerConfig, newClientFn); err != nil {
		return nil, err
	}

	return f, nil
}

func (f *Factory) CreateTraceReader() (tracestore.Reader, error) {
	return NewTraceReader(f.readerConn), nil
}

func (f *Factory) CreateTraceWriter() (tracestore.Writer, error) {
	return NewTraceWriter(f.writerConn), nil
}

func (f *Factory) CreateDependencyReader() (depstore.Reader, error) {
	return NewDependencyReader(f.readerConn), nil
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

type newClientFn func(telset component.TelemetrySettings, gcs *configgrpc.ClientConfig, opts ...grpc.DialOption) (*grpc.ClientConn, error)

func (f *Factory) initializeConnections(
	readerTelset, writerTelset component.TelemetrySettings,
	readerConfig, writerConfig *configgrpc.ClientConfig,
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

	createConn := func(telset component.TelemetrySettings, gcs *configgrpc.ClientConfig) (*grpc.ClientConn, error) {
		opts := append(baseOpts, grpc.WithStatsHandler(
			otelgrpc.NewClientHandler(otelgrpc.WithTracerProvider(telset.TracerProvider)),
		))
		return newClient(telset, gcs, opts...)
	}

	readerConn, err := createConn(readerTelset, readerConfig)
	if err != nil {
		return fmt.Errorf("error creating reader client connection: %w", err)
	}
	writerConn, err := createConn(writerTelset, writerConfig)
	if err != nil {
		return fmt.Errorf("error creating writer client connection: %w", err)
	}

	f.readerConn, f.writerConn = readerConn, writerConn

	return nil
}
