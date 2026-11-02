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
	"google.golang.org/grpc/connectivity"
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
	lis := serveListener(t)

	cfg := Config{
		ClientConfig: configgrpc.ClientConfig{
			Endpoint: lis.Addr().String(),
			TLS:      configtls.ClientConfig{Insecure: true},
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
	waitReady(t, f.readerConn, f.writerConn)
	require.Equal(t, lis.Addr().String(), f.readerConn.Target())
	require.Equal(t, lis.Addr().String(), f.writerConn.Target())
}

func TestNewFactory_WriteEndpointOverride(t *testing.T) {
	readListener := serveListener(t)
	writeListener := serveListener(t)

	cfg := Config{
		ClientConfig: configgrpc.ClientConfig{
			Endpoint: readListener.Addr().String(),
			TLS:      configtls.ClientConfig{Insecure: true},
		},
		Writer: configgrpc.ClientConfig{
			Endpoint: writeListener.Addr().String(),
			TLS:      configtls.ClientConfig{Insecure: true},
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
	waitReady(t, f.readerConn, f.writerConn)
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
	lis := serveListener(t)

	cfg := Config{
		ClientConfig: configgrpc.ClientConfig{
			Endpoint: lis.Addr().String(),
			TLS:      configtls.ClientConfig{Insecure: true},
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
	waitReady(t, f.readerConn, f.writerConn)
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
	f := &Factory{}
	newClientFn := func(_ component.TelemetrySettings, _ *configgrpc.ClientConfig, _ ...grpc.DialOption) (conn *grpc.ClientConn, err error) {
		return nil, assert.AnError
	}
	noopTelset := telemetry.NoopSettings().ToOtelComponent()
	err := f.initializeConnections(
		noopTelset,
		noopTelset,
		&configgrpc.ClientConfig{},
		&configgrpc.ClientConfig{},
		newClientFn,
	)
	assert.ErrorContains(t, err, "error creating reader client connection")
}

// serveListener returns a listener with an empty gRPC server accepting on it, because
// configgrpc.ToClientConn connects eagerly and a connection to an unserved listener is
// still mid-dial when Factory.Close runs, leaking TCP dial goroutines past the package
// leak check. The connection reaches Ready only with an insecure client config as well,
// since the default client expects TLS.
func serveListener(t *testing.T) net.Listener {
	t.Helper()
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err, "failed to listen")
	server := grpc.NewServer()
	go func() { server.Serve(lis) }()
	t.Cleanup(server.Stop)
	return lis
}

// waitReady blocks until every connection has finished connecting, so that closing
// it tears down an established transport instead of an in-flight dial.
func waitReady(t *testing.T, conns ...*grpc.ClientConn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, conn := range conns {
		for state := conn.GetState(); state != connectivity.Ready; state = conn.GetState() {
			require.True(t, conn.WaitForStateChange(ctx, state), "connection stuck in state %s", state)
		}
	}
}

// TestNewFactory_TracesReadsNotWrites checks that the reader connection is
// instrumented with the TracerProvider it is given, that the writer connection stays
// uninstrumented, and that one RPC produces one client span parented to its caller.
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
	invoke := func(ctx context.Context, conn *grpc.ClientConn, method string) {
		err := conn.Invoke(ctx, method, &emptypb.Empty{}, &emptypb.Empty{}, grpc.WaitForReady(true))
		require.Equal(t, codes.Unimplemented, status.Code(err))
	}
	callerCtx, caller := tp.Tracer("test").Start(ctx, "caller")
	invoke(callerCtx, f.readerConn, readMethod)
	caller.End()
	invoke(ctx, f.writerConn, writeMethod)

	mu.Lock()
	defer mu.Unlock()
	assert.NotEmpty(t, captured[readMethod].Get("traceparent"), "read RPC must propagate trace context")
	assert.Empty(t, captured[writeMethod].Get("traceparent"), "write RPC must not be traced")

	// The read RPC and the caller are the only spans: a traced write would add a third.
	// The read span has to name the caller as its parent, because a connection carrying
	// two otelgrpc stats handlers starts a second span inside the first and then ends
	// only the inner one, leaving this span pointing at a parent that is never exported.
	ended := recorder.Ended()
	require.Len(t, ended, 2)
	assert.Equal(t, readMethod[1:], ended[0].Name())
	assert.Equal(t, caller.SpanContext().SpanID(), ended[0].Parent().SpanID())
}

func TestNewFactory_Timeout(t *testing.T) {
	const (
		readMethod   = "/jaeger.storage.v2.TraceReader/GetServices"
		streamMethod = "/jaeger.storage.v2.TraceReader/GetTraces"
		writeMethod  = "/opentelemetry.proto.collector.trace.v1.TraceService/Export"
	)
	tests := []struct {
		name    string
		timeout time.Duration
	}{
		{name: "configured", timeout: time.Minute},
		{name: "disabled", timeout: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			hasDeadline := make(map[string]bool)
			server := grpc.NewServer(grpc.UnknownServiceHandler(
				func(_ any, stream grpc.ServerStream) error {
					method, _ := grpc.MethodFromServerStream(stream)
					_, ok := stream.Context().Deadline()
					mu.Lock()
					hasDeadline[method] = ok
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
				TimeoutConfig: exporterhelper.TimeoutConfig{Timeout: tt.timeout},
			}
			f, err := NewFactory(context.Background(), cfg, telemetry.NoopSettings())
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, f.Close()) })
			waitReady(t, f.readerConn, f.writerConn)

			// The caller's context carries no deadline, so any deadline the server
			// sees comes from the configured timeout.
			ctx := context.Background()
			for method, conn := range map[string]*grpc.ClientConn{readMethod: f.readerConn, writeMethod: f.writerConn} {
				err := conn.Invoke(ctx, method, &emptypb.Empty{}, &emptypb.Empty{})
				require.Equal(t, codes.Unimplemented, status.Code(err))
			}
			stream, err := f.readerConn.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true}, streamMethod)
			require.NoError(t, err)
			require.NoError(t, stream.SendMsg(&emptypb.Empty{}))
			require.NoError(t, stream.CloseSend())
			require.Equal(t, codes.Unimplemented, status.Code(stream.RecvMsg(&emptypb.Empty{})))

			mu.Lock()
			defer mu.Unlock()
			assert.Equal(t, tt.timeout > 0, hasDeadline[readMethod], "read call deadline")
			assert.Equal(t, tt.timeout > 0, hasDeadline[writeMethod], "write call deadline")
			assert.False(t, hasDeadline[streamMethod], "streaming calls are not bounded by the timeout")
		})
	}
}

func TestTimeoutUnaryClientInterceptor_DeadlineExceeded(t *testing.T) {
	interceptor := timeoutUnaryClientInterceptor(10 * time.Millisecond)
	slowInvoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		<-ctx.Done()
		return status.FromContextError(ctx.Err()).Err()
	}
	err := interceptor(context.Background(), "/test/Slow", nil, nil, nil, slowInvoker)
	assert.Equal(t, codes.DeadlineExceeded, status.Code(err))
}
