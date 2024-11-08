// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/configtelemetry"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/pkg/bearertoken"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/plugin"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var ( // interface comformance checks
	_ storage.Factory        = (*Factory)(nil)
	_ storage.ArchiveFactory = (*Factory)(nil)
	_ io.Closer              = (*Factory)(nil)
	_ plugin.Configurable    = (*Factory)(nil)
)

// Factory implements storage.Factory and creates storage components backed by a storage plugin.
type Factory struct {
	metricsFactory     metrics.Factory
	logger             *zap.Logger
	tracerProvider     trace.TracerProvider
	config             Config
	services           *ClientPluginServices
	tracedRemoteConn   *grpc.ClientConn
	untracedRemoteConn *grpc.ClientConn
	host               component.Host
	meterProvider      metric.MeterProvider
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{
		host: componenttest.NewNopHost(),
	}
}

// NewFactoryWithConfig is used from jaeger(v2).
func NewFactoryWithConfig(
	cfg Config,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
	host component.Host,
	meterProvider metric.MeterProvider,
) (*Factory, error) {
	f := NewFactory()
	f.config = cfg
	f.host = host
	f.meterProvider = meterProvider
	if err := f.Initialize(metricsFactory, logger); err != nil {
		return nil, err
	}
	return f, nil
}

// AddFlags implements plugin.Configurable
func (*Factory) AddFlags(flagSet *flag.FlagSet) {
	addFlags(flagSet)
}

// InitFromViper implements plugin.Configurable
func (f *Factory) InitFromViper(v *viper.Viper, logger *zap.Logger) {
	if err := initFromViper(&f.config, v); err != nil {
		logger.Fatal("unable to initialize gRPC storage factory", zap.Error(err))
	}
}

// Initialize implements storage.Factory
func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.metricsFactory, f.logger = metricsFactory, logger
	f.tracerProvider = otel.GetTracerProvider()

	tracedTelset := getTelset(logger, f.tracerProvider, f.meterProvider)
	untracedTelset := getTelset(logger, noop.NewTracerProvider(), f.meterProvider)
	newClientFn := func(telset component.TelemetrySettings, opts ...grpc.DialOption) (conn *grpc.ClientConn, err error) {
		clientOpts := make([]configgrpc.ToClientConnOption, 0)
		for _, opt := range opts {
			clientOpts = append(clientOpts, configgrpc.WithGrpcDialOption(opt))
		}
		return f.config.ToClientConn(context.Background(), f.host, telset, clientOpts...)
	}

	var err error
	f.services, err = f.newRemoteStorage(tracedTelset, untracedTelset, newClientFn)
	if err != nil {
		return fmt.Errorf("grpc storage builder failed to create a store: %w", err)
	}
	logger.Info("Remote storage configuration", zap.Any("configuration", f.config))
	return nil
}

type newClientFn func(telset component.TelemetrySettings, opts ...grpc.DialOption) (*grpc.ClientConn, error)

func (f *Factory) newRemoteStorage(
	tracedTelset component.TelemetrySettings,
	untracedTelset component.TelemetrySettings,
	newClient newClientFn,
) (*ClientPluginServices, error) {
	c := f.config
	if c.Auth != nil {
		return nil, errors.New("authenticator is not supported")
	}
	unaryInterceptors := []grpc.UnaryClientInterceptor{
		bearertoken.NewUnaryClientInterceptor(),
	}
	streamInterceptors := []grpc.StreamClientInterceptor{
		bearertoken.NewStreamClientInterceptor(),
	}
	tenancyMgr := tenancy.NewManager(&c.Tenancy)
	if tenancyMgr.Enabled {
		unaryInterceptors = append(unaryInterceptors, tenancy.NewClientUnaryInterceptor(tenancyMgr))
		streamInterceptors = append(streamInterceptors, tenancy.NewClientStreamInterceptor(tenancyMgr))
	}

	baseOpts := append(
		[]grpc.DialOption{},
		grpc.WithChainUnaryInterceptor(unaryInterceptors...),
		grpc.WithChainStreamInterceptor(streamInterceptors...),
	)
	opts := append([]grpc.DialOption{}, baseOpts...)
	opts = append(opts, grpc.WithStatsHandler(otelgrpc.NewClientHandler(otelgrpc.WithTracerProvider(tracedTelset.TracerProvider))))

	tracedRemoteConn, err := newClient(tracedTelset, opts...)
	if err != nil {
		return nil, fmt.Errorf("error creating traced remote storage client: %w", err)
	}
	f.tracedRemoteConn = tracedRemoteConn
	untracedOpts := append([]grpc.DialOption{}, baseOpts...)
	untracedOpts = append(
		untracedOpts,
		grpc.WithStatsHandler(
			otelgrpc.NewClientHandler(
				otelgrpc.WithTracerProvider(untracedTelset.TracerProvider))))
	untracedRemoteConn, err := newClient(tracedTelset, untracedOpts...)
	if err != nil {
		return nil, fmt.Errorf("error creating untraced remote storage client: %w", err)
	}
	f.untracedRemoteConn = untracedRemoteConn
	grpcClient := shared.NewGRPCClient(tracedRemoteConn, untracedRemoteConn)
	return &ClientPluginServices{
		PluginServices: shared.PluginServices{
			Store:               grpcClient,
			ArchiveStore:        grpcClient,
			StreamingSpanWriter: grpcClient,
		},
		Capabilities: grpcClient,
	}, nil
}

// CreateSpanReader implements storage.Factory
func (f *Factory) CreateSpanReader() (spanstore.Reader, error) {
	return f.services.Store.SpanReader(), nil
}

// CreateSpanWriter implements storage.Factory
func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
	if f.services.Capabilities != nil && f.services.StreamingSpanWriter != nil {
		if capabilities, err := f.services.Capabilities.Capabilities(); err == nil && capabilities.StreamingSpanWriter {
			return f.services.StreamingSpanWriter.StreamingSpanWriter(), nil
		}
	}
	return f.services.Store.SpanWriter(), nil
}

// CreateDependencyReader implements storage.Factory
func (f *Factory) CreateDependencyReader() (dependencystore.Reader, error) {
	return f.services.Store.DependencyReader(), nil
}

// CreateArchiveSpanReader implements storage.ArchiveFactory
func (f *Factory) CreateArchiveSpanReader() (spanstore.Reader, error) {
	if f.services.Capabilities == nil {
		return nil, storage.ErrArchiveStorageNotSupported
	}
	capabilities, err := f.services.Capabilities.Capabilities()
	if err != nil {
		return nil, err
	}
	if capabilities == nil || !capabilities.ArchiveSpanReader {
		return nil, storage.ErrArchiveStorageNotSupported
	}
	return f.services.ArchiveStore.ArchiveSpanReader(), nil
}

// CreateArchiveSpanWriter implements storage.ArchiveFactory
func (f *Factory) CreateArchiveSpanWriter() (spanstore.Writer, error) {
	if f.services.Capabilities == nil {
		return nil, storage.ErrArchiveStorageNotSupported
	}
	capabilities, err := f.services.Capabilities.Capabilities()
	if err != nil {
		return nil, err
	}
	if capabilities == nil || !capabilities.ArchiveSpanWriter {
		return nil, storage.ErrArchiveStorageNotSupported
	}
	return f.services.ArchiveStore.ArchiveSpanWriter(), nil
}

// Close closes the resources held by the factory
func (f *Factory) Close() error {
	var errs []error
	if f.tracedRemoteConn != nil {
		errs = append(errs, f.tracedRemoteConn.Close())
	}
	if f.untracedRemoteConn != nil {
		errs = append(errs, f.untracedRemoteConn.Close())
	}
	return errors.Join(errs...)
}

func getTelset(logger *zap.Logger, tracerProvider trace.TracerProvider, meterProvider metric.MeterProvider) component.TelemetrySettings {
	return component.TelemetrySettings{
		Logger:         logger,
		TracerProvider: tracerProvider,
		// TODO needs to be joined with the metricsFactory
		LeveledMeterProvider: func(_ configtelemetry.Level) metric.MeterProvider {
			return meterProvider
		},
	}
}
