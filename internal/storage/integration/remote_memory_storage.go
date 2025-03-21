// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"os"
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
	"github.com/jaegertracing/jaeger/internal/config"
	"github.com/jaegertracing/jaeger/internal/healthcheck"
	storage "github.com/jaegertracing/jaeger/internal/storage/v1/factory"
	"github.com/jaegertracing/jaeger/internal/tenancy"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/telemetry"
	"github.com/jaegertracing/jaeger/ports"
)

type RemoteMemoryStorage struct {
	server         *app.Server
	storageFactory *storage.Factory
}

func StartNewRemoteMemoryStorage(t *testing.T, port int) *RemoteMemoryStorage {
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
	opts := &app.Options{
		ServerConfig: configgrpc.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint: ports.PortToHostPort(port),
			},
		},
		Tenancy: tenancy.Options{
			Enabled: false,
		},
	}
	tm := tenancy.NewManager(&opts.Tenancy)
	storageFactory, err := storage.NewFactory(storage.ConfigFromEnvAndCLI(os.Args, os.Stderr))
	require.NoError(t, err)

	v, _ := config.Viperize(storageFactory.AddFlags)
	storageFactory.InitFromViper(v, logger)
	require.NoError(t, storageFactory.Initialize(metrics.NullFactory, logger))

	t.Logf("Starting in-process remote storage server on %s", opts.NetAddr.Endpoint)
	telset := telemetry.NoopSettings()
	telset.Logger = logger
	telset.ReportStatus = telemetry.HCAdapter(healthcheck.New())
	server, err := app.NewServer(opts, storageFactory, tm, telset)
	require.NoError(t, err)
	require.NoError(t, server.Start())

	conn, err := grpc.NewClient(
		opts.NetAddr.Endpoint,
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
		storageFactory: storageFactory,
	}
}

func (s *RemoteMemoryStorage) Close(t *testing.T) {
	require.NoError(t, s.server.Close())
	require.NoError(t, s.storageFactory.Close())
}
