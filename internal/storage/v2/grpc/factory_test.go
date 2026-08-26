// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"math"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configauth"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/jaegertracing/jaeger/internal/headerforwarding"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

func TestNewFactory_InvalidMaxRecvMsgSize(t *testing.T) {
	for _, v := range []int{-1, math.MaxInt} {
		cfg := &Config{MaxRecvMsgSizeMiB: v}
		_, err := NewFactory(context.Background(), *cfg, telemetry.NoopSettings())
		require.ErrorContains(t, err, "max_recv_msg_size_mib must be between 0 and")
	}
}

func TestNewFactory_NonEmptyAuthenticator(t *testing.T) {
	cfg := &Config{
		ClientConfig: configgrpc.ClientConfig{
			Auth: configoptional.Some(configauth.Config{}),
		},
	}
	_, err := NewFactory(context.Background(), *cfg, telemetry.NoopSettings())
	require.ErrorContains(t, err, "authenticator is not supported")
}

func TestNewFactory(t *testing.T) {
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err, "failed to listen")
	t.Cleanup(func() { require.NoError(t, lis.Close()) })

	cfg := Config{
		ClientConfig: configgrpc.ClientConfig{
			Endpoint: lis.Addr().String(),
		},
		TimeoutConfig: exporterhelper.TimeoutConfig{
			Timeout: 1 * time.Second,
		},
		Tenancy: tenancy.Options{
			Enabled: true,
		},
	}
	telset := telemetry.NoopSettings()
	f, err := NewFactory(context.Background(), cfg, telset)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })
	require.Equal(t, lis.Addr().String(), f.readerConn.Target())
	require.Equal(t, lis.Addr().String(), f.writerConn.Target())
}

func TestNewFactory_WriteEndpointOverride(t *testing.T) {
	readListener, err := net.Listen("tcp", ":0")
	require.NoError(t, err, "failed to listen")
	t.Cleanup(func() { require.NoError(t, readListener.Close()) })

	writeListener, err := net.Listen("tcp", ":0")
	require.NoError(t, err, "failed to listen")
	t.Cleanup(func() { require.NoError(t, writeListener.Close()) })

	cfg := Config{
		ClientConfig: configgrpc.ClientConfig{
			Endpoint: readListener.Addr().String(),
		},
		Writer: configgrpc.ClientConfig{
			Endpoint: writeListener.Addr().String(),
		},
		TimeoutConfig: exporterhelper.TimeoutConfig{
			Timeout: 1 * time.Second,
		},
		Tenancy: tenancy.Options{
			Enabled: true,
		},
	}
	telset := telemetry.NoopSettings()
	f, err := NewFactory(context.Background(), cfg, telset)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })
	require.Equal(t, readListener.Addr().String(), f.readerConn.Target())
	require.Equal(t, writeListener.Addr().String(), f.writerConn.Target())
}

func TestFactory(t *testing.T) {
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err, "failed to listen")

	s := grpc.NewServer()

	conn := startServer(t, s, lis)
	f := &Factory{
		readerConn: conn,
	}

	t.Run("CreateTraceReader", func(t *testing.T) {
		tr, err := f.CreateTraceReader()
		require.NoError(t, err)
		require.NotNil(t, tr)
	})

	t.Run("CreateTraceWriter", func(t *testing.T) {
		tr, err := f.CreateTraceWriter()
		require.NoError(t, err)
		require.NotNil(t, tr)
	})

	t.Run("CreateDependencyReader", func(t *testing.T) {
		tr, err := f.CreateDependencyReader()
		require.NoError(t, err)
		require.NotNil(t, tr)
	})
}

func TestNewFactory_WithHeaderForwarding(t *testing.T) {
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err, "failed to listen")
	t.Cleanup(func() { require.NoError(t, lis.Close()) })

	cfg := Config{
		ClientConfig: configgrpc.ClientConfig{
			Endpoint: lis.Addr().String(),
		},
		TimeoutConfig: exporterhelper.TimeoutConfig{
			Timeout: 1 * time.Second,
		},
		HeaderForwarding: []headerforwarding.ForwardedHeader{
			{HTTPName: "x-user", Role: headerforwarding.RoleUsername},
		},
	}
	f, err := NewFactory(context.Background(), cfg, telemetry.NoopSettings())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })
	require.Equal(t, lis.Addr().String(), f.readerConn.Target())
}

func TestNewFactory_MaxRecvMsgSize(t *testing.T) {
	capture := func(cfg Config) []grpc.DialOption {
		var captured []grpc.DialOption
		f := &Factory{config: cfg}
		noopTelset := telemetry.NoopSettings().ToOtelComponent()
		_ = f.initializeConnections(
			noopTelset, noopTelset,
			&cfg.ClientConfig, &cfg.ClientConfig,
			func(_ component.TelemetrySettings, _ *configgrpc.ClientConfig, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
				captured = opts
				return nil, assert.AnError // stop after first capture; error is ignored by caller
			},
		)
		return captured
	}

	base := Config{ClientConfig: configgrpc.ClientConfig{Endpoint: "localhost:0"}}
	withSize := base
	withSize.MaxRecvMsgSizeMiB = 16

	optsBase := capture(base)
	optsWithSize := capture(withSize)

	assert.Greater(t, len(optsWithSize), len(optsBase), "MaxRecvMsgSizeMiB > 0 should add a WithDefaultCallOptions dial option")
}

func TestInitializeConnections_ClientError(t *testing.T) {
	f, err := NewFactory(
		context.Background(),
		Config{
			ClientConfig: configgrpc.ClientConfig{
				Endpoint: ":0",
			},
		}, telemetry.NoopSettings(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })
	newClientFn := func(_ component.TelemetrySettings, _ *configgrpc.ClientConfig, _ ...grpc.DialOption) (conn *grpc.ClientConn, err error) {
		return nil, assert.AnError
	}
	noopTelset := telemetry.NoopSettings().ToOtelComponent()
	err = f.initializeConnections(
		noopTelset,
		noopTelset,
		&configgrpc.ClientConfig{},
		&configgrpc.ClientConfig{},
		newClientFn,
	)
	assert.ErrorContains(t, err, "error creating reader client connection")
}

// TestNewFactory_TracesReadsNotWrites checks that the reader connection is
// instrumented with the TracerProvider it is given and that the writer connection
// stays uninstrumented.
func TestNewFactory_TracesReadsNotWrites(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { require.NoError(t, tp.Shutdown(context.Background())) })
	// otelgrpc injects the outgoing trace context with the global propagator,
	// which jtracer installs in production.
	otel.SetTextMapPropagator(propagation.TraceContext{})

	const (
		readMethod  = "/jaeger.storage.v2.TraceReader/GetServices"
		writeMethod = "/opentelemetry.proto.collector.trace.v1.TraceService/Export"
	)

	var mu sync.Mutex
	captured := make(map[string]metadata.MD)
	server := grpc.NewServer(grpc.UnknownServiceHandler(
		func(_ any, stream grpc.ServerStream) error {
			method, _ := grpc.MethodFromServerStream(stream)
			md, _ := metadata.FromIncomingContext(stream.Context())
			mu.Lock()
			captured[method] = md
			mu.Unlock()
			return status.Error(codes.Unimplemented, "test server implements no services")
		},
	))
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	go func() { server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		listener.Close()
	})

	cfg := Config{
		ClientConfig: configgrpc.ClientConfig{
			Endpoint: listener.Addr().String(),
			TLS:      configtls.ClientConfig{Insecure: true},
		},
	}
	telset := telemetry.NoopSettings()
	telset.TracerProvider = tp
	f, err := NewFactory(context.Background(), cfg, telset)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	invoke := func(conn *grpc.ClientConn, method string) {
		err := conn.Invoke(ctx, method, &emptypb.Empty{}, &emptypb.Empty{}, grpc.WaitForReady(true))
		require.Equal(t, codes.Unimplemented, status.Code(err))
	}
	invoke(f.readerConn, readMethod)
	invoke(f.writerConn, writeMethod)

	mu.Lock()
	defer mu.Unlock()
	assert.NotEmpty(t, captured[readMethod].Get("traceparent"), "read RPC must propagate trace context")
	assert.Empty(t, captured[writeMethod].Get("traceparent"), "write RPC must not be traced")

	var spanNames []string
	for _, span := range recorder.Ended() {
		spanNames = append(spanNames, span.Name())
	}
	assert.Equal(t, []string{readMethod[1:]}, spanNames)
}
