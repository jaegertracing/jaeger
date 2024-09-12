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
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"google.golang.org/grpc"

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
	metricsFactory metrics.Factory
	logger         *zap.Logger
	tracerProvider trace.TracerProvider

	// configV1 is used for backward compatibility. it will be removed in v2.
	// In the main initialization logic, only configV2 is used.
	configV1 Configuration
	configV2 *ConfigV2

	services    *ClientPluginServices
	remoteConns []*grpc.ClientConn
}

// NewFactory creates a new Factory.
func NewFactory() *Factory {
	return &Factory{}
}

// NewFactoryWithConfig is used from jaeger(v2).
func NewFactoryWithConfig(
	cfg ConfigV2,
	metricsFactory metrics.Factory,
	logger *zap.Logger,
) (*Factory, error) {
	f := NewFactory()
	f.configV2 = &cfg
	if err := f.Initialize(metricsFactory, logger); err != nil {
		return nil, err
	}
	return f, nil
}

// AddFlags implements plugin.Configurable
func (*Factory) AddFlags(flagSet *flag.FlagSet) {
	v1AddFlags(flagSet)
}

// InitFromViper implements plugin.Configurable
func (f *Factory) InitFromViper(v *viper.Viper, logger *zap.Logger) {
	if err := v1InitFromViper(&f.configV1, v); err != nil {
		logger.Fatal("unable to initialize gRPC storage factory", zap.Error(err))
	}
}

// Initialize implements storage.Factory
func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	f.metricsFactory, f.logger = metricsFactory, logger
	f.tracerProvider = otel.GetTracerProvider()

	if f.configV2 == nil {
		f.configV2 = f.configV1.TranslateToConfigV2()
	}

	telset := component.TelemetrySettings{
		Logger: logger,
		// TODO needs to be joined with the metricsFactory
		LeveledMeterProvider: func(_ configtelemetry.Level) metric.MeterProvider {
			return noopmetric.NewMeterProvider()
		},
	}
	newClientFn := func(telSettings component.TelemetrySettings, opts ...grpc.DialOption) (conn *grpc.ClientConn, err error) {
		clientOpts := make([]configgrpc.ToClientConnOption, 0)
		for _, opt := range opts {
			clientOpts = append(clientOpts, configgrpc.WithGrpcDialOption(opt))
		}
		return f.configV2.ToClientConnWithOptions(context.Background(), componenttest.NewNopHost(), telSettings, clientOpts...)
	}

	var err error
	f.services, err = f.newRemoteStorage(telset, newClientFn)
	if err != nil {
		return fmt.Errorf("grpc storage builder failed to create a store: %w", err)
	}
	logger.Info("Remote storage configuration", zap.Any("configuration", f.configV2))
	return nil
}

type newClientFn func(telSettings component.TelemetrySettings, opts ...grpc.DialOption) (*grpc.ClientConn, error)

func (f *Factory) newRemoteStorage(telset component.TelemetrySettings, newClient newClientFn) (*ClientPluginServices, error) {
	c := f.configV2
	opts := make([]grpc.DialOption, 0)
	if c.Auth != nil {
		return nil, fmt.Errorf("authenticator is not supported")
	}

	tenancyMgr := tenancy.NewManager(&c.Tenancy)
	if tenancyMgr.Enabled {
		opts = append(opts, grpc.WithUnaryInterceptor(tenancy.NewClientUnaryInterceptor(tenancyMgr)))
		opts = append(opts, grpc.WithStreamInterceptor(tenancy.NewClientStreamInterceptor(tenancyMgr)))
	}

	noTraceConn, err := newClient(telset, opts...)
	if err != nil {
		return nil, fmt.Errorf("error creating remote storage client without tracing: %w", err)
	}
	f.remoteConns = append(f.remoteConns, noTraceConn)

	opts = append(opts, grpc.WithStatsHandler(otelgrpc.NewClientHandler(otelgrpc.WithTracerProvider(f.tracerProvider))))
	traceConn, err := newClient(telset, opts...)
	if err != nil {
		return nil, fmt.Errorf("error creating remote storage client with tracing: %w", err)
	}
	f.remoteConns = append(f.remoteConns, traceConn)

	grpcClient := shared.NewGRPCClient(traceConn, noTraceConn)
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
	for i := range f.remoteConns {
		errs = append(errs, f.remoteConns[i].Close())
	}
	errs = append(errs, f.configV1.RemoteTLS.Close())
	return errors.Join(errs...)
}
