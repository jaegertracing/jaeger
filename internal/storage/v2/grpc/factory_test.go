// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"math"
	"net"
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
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/internal/headerforwarding"
	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/proto-gen/storage/v2"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
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

type sleepServer struct {
	storage.UnimplementedTraceReaderServer
	sleepDuration time.Duration
}

func (s *sleepServer) GetServices(ctx context.Context, _ *storage.GetServicesRequest) (*storage.GetServicesResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(s.sleepDuration):
		return &storage.GetServicesResponse{Services: []string{"foo"}}, nil
	}
}

func (s *sleepServer) GetTraces(_ *storage.GetTracesRequest, srv storage.TraceReader_GetTracesServer) error {
	select {
	case <-srv.Context().Done():
		return srv.Context().Err()
	case <-time.After(s.sleepDuration):
		return nil
	}
}

func TestFactory_Timeout(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	s := grpc.NewServer()
	srv := &sleepServer{sleepDuration: 200 * time.Millisecond}
	storage.RegisterTraceReaderServer(s, srv)

	go func() {
		_ = s.Serve(lis)
	}()
	defer s.Stop()

	cfg := Config{
		ClientConfig: configgrpc.ClientConfig{
			Endpoint: lis.Addr().String(),
			TLS: configtls.ClientConfig{
				Insecure: true,
			},
		},
		TimeoutConfig: exporterhelper.TimeoutConfig{
			Timeout: 20 * time.Millisecond,
		},
	}

	f, err := NewFactory(context.Background(), cfg, telemetry.NoopSettings())
	require.NoError(t, err)
	defer f.Close()

	tr, err := f.CreateTraceReader()
	require.NoError(t, err)

	t.Run("unary timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_, err := tr.GetServices(ctx)
		require.Error(t, err)
		// gRPC translates context deadline exceeded into codes.DeadlineExceeded;
		// check the status code rather than the error string to be format-agnostic.
		assert.Equal(t, codes.DeadlineExceeded, status.Code(err))
	})

	t.Run("streaming timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		it := tr.GetTraces(ctx, tracestore.GetTraceParams{})
		_, err := jiter.FlattenWithErrors(it)
		require.Error(t, err)
		// gRPC translates context deadline exceeded into codes.DeadlineExceeded;
		// check the status code rather than the error string to be format-agnostic.
		assert.Equal(t, codes.DeadlineExceeded, status.Code(err))
	})
}
