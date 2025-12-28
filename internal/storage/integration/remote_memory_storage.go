// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confignet"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/jaegertracing/jaeger/cmd/remote-storage/app"
	"github.com/jaegertracing/jaeger/internal/healthcheck"
	"github.com/jaegertracing/jaeger/internal/storage/v2/memory"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
	"github.com/jaegertracing/jaeger/ports"
)

type RemoteMemoryStorage struct {
	server         *app.Server
	storageFactory *memory.Factory
}

func StartNewRemoteMemoryStorage(port int, logger *zap.Logger) (*RemoteMemoryStorage, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	grpcCfg := configgrpc.ServerConfig{
		NetAddr: confignet.AddrConfig{
			Endpoint: ports.PortToHostPort(port),
		},
	}

	tm := tenancy.NewManager(&tenancy.Options{Enabled: false})

	logger.Info("starting in-process remote storage server",
		zap.String("endpoint", grpcCfg.NetAddr.Endpoint),
	)

	telset := telemetry.NoopSettings()
	telset.Logger = logger
	telset.ReportStatus = telemetry.HCAdapter(healthcheck.New())

	traceFactory, err := memory.NewFactory(
		memory.Configuration{MaxTraces: 10000},
		telset,
	)
	if err != nil {
		return nil, err
	}

	server, err := app.NewServer(
		context.Background(),
		grpcCfg,
		traceFactory,
		traceFactory,
		tm,
		telset,
	)
	if err != nil {
		return nil, err
	}

	if err := server.Start(context.Background()); err != nil {
		return nil, err
	}

	conn, err := grpc.NewClient(
		grpcCfg.NetAddr.Endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	healthClient := grpc_health_v1.NewHealthClient(conn)

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		resp, err := healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
		cancel()

		if err == nil && resp.GetStatus() == grpc_health_v1.HealthCheckResponse_SERVING {
			logger.Info("remote storage server is ready")
			return &RemoteMemoryStorage{
				server:         server,
				storageFactory: traceFactory,
			}, nil
		}

		logger.Debug("remote storage server not ready yet", zap.Error(err))
		time.Sleep(time.Second)
	}

	return nil, fmt.Errorf("remote storage server did not become ready in time")
}

func (s *RemoteMemoryStorage) Close() error {
	return s.server.Close()
}
