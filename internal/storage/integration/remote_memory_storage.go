// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confignet"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/jaegertracing/jaeger/cmd/remote-storage/app"
	"github.com/jaegertracing/jaeger/internal/storage/v2/memory"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
	"github.com/jaegertracing/jaeger/ports"
)

type RemoteMemoryStorage struct {
	server         *app.Server
	storageFactory *memory.Factory
	hcHost         *telemetry.HealthCheckHost
}

func StartNewRemoteMemoryStorage(t *testing.T, port int) *RemoteMemoryStorage {
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
	grpcCfg := configgrpc.ServerConfig{
		NetAddr: confignet.AddrConfig{
			Endpoint: ports.PortToHostPort(port),
		},
	}
	tm := tenancy.NewManager(&tenancy.Options{
		Enabled: false,
	})

	t.Logf("Starting in-process remote storage server on %s", grpcCfg.NetAddr.Endpoint)
	telset := telemetry.NoopSettings()
	telset.Logger = logger

	// Create health check host on a unique port for tests
	hcHost, err := telemetry.NewHealthCheckHost(
		context.Background(),
		telset.ToOtelComponent(),
		fmt.Sprintf(":%d", port+1000), // offset to avoid conflicts
	)
	require.NoError(t, err)
	require.NoError(t, hcHost.Start(context.Background()))
	telset.Host = hcHost

	traceFactory, err := memory.NewFactory(
		memory.Configuration{
			MaxTraces: 10000,
		},
		telset,
	)
	require.NoError(t, err)

	server, err := app.NewServer(context.Background(), grpcCfg, traceFactory, traceFactory, tm, telset)
	require.NoError(t, err)
	require.NoError(t, server.Start(context.Background()))

	conn, err := grpc.NewClient(
		grpcCfg.NetAddr.Endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()
	healthClient := grpc_health_v1.NewHealthClient(conn)
	require.Eventually(t, func() bool {
		req := &grpc_health_v1.HealthCheckRequest{}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
		defer cancel()
		resp, err := healthClient.Check(ctx, req)
		if err != nil {
			t.Logf("remote storage server is not ready: err=%v", err)
			return false
		}
		t.Logf("remote storage server status: %v", resp.Status)
		return resp.GetStatus() == grpc_health_v1.HealthCheckResponse_SERVING
	}, 30*time.Second, time.Second, "failed to ensure remote storage server is ready")

	return &RemoteMemoryStorage{
		server:         server,
		storageFactory: traceFactory,
		hcHost:         hcHost,
	}
}

func (s *RemoteMemoryStorage) Close(t *testing.T) {
	require.NoError(t, s.server.Close())
	require.NoError(t, s.hcHost.Shutdown(context.Background()))
}
