// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/remote-storage/app"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/plugin/storage"
	"github.com/jaegertracing/jaeger/ports"
)

type GRPCStorageIntegration struct {
	E2EStorageIntegration

	logger         *zap.Logger
	server         *app.Server
	storageFactory *storage.Factory
}

func (s *GRPCStorageIntegration) initialize() error {
	err := s.startServer()
	if err != nil {
		return err
	}

	s.Refresh = s.refresh
	s.CleanUp = s.cleanUp
	return nil
}

func (s *GRPCStorageIntegration) startServer() error {
	opts := &app.Options{
		GRPCHostPort: ports.PortToHostPort(ports.RemoteStorageGRPC),
		Tenancy: tenancy.Options{
			Enabled: false,
		},
	}
	tm := tenancy.NewManager(&opts.Tenancy)
	var err error
	s.storageFactory, err = storage.NewFactory(storage.FactoryConfigFromEnvAndCLI(os.Args, os.Stderr))
	if err != nil {
		return err
	}
	v, _ := config.Viperize(s.storageFactory.AddFlags)
	s.storageFactory.InitFromViper(v, s.logger)
	err = s.storageFactory.Initialize(metrics.NullFactory, s.logger)
	if err != nil {
		return err
	}

	s.server, err = app.NewServer(opts, s.storageFactory, tm, s.logger, healthcheck.New())
	if err != nil {
		return err
	}
	err = s.server.Start()
	if err != nil {
		return err
	}
	return nil
}

func (s *GRPCStorageIntegration) refresh() error {
	return nil
}

func (s *GRPCStorageIntegration) cleanUp() error {
	if err := s.server.Close(); err != nil {
		return err
	}
	if err := s.storageFactory.Close(); err != nil {
		return err
	}
	return s.initialize()
}

func TestGRPCStorage(t *testing.T) {
	if os.Getenv("STORAGE") != "grpc" {
		t.Skip("Integration test against gRPC skipped; set STORAGE env var to grpc to run this")
	}

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	s := &GRPCStorageIntegration{
		logger: logger,
	}
	require.NoError(t, s.initialize())
	require.NoError(t, s.e2eInitialize())
	t.Cleanup(func() {
		require.NoError(t, s.e2eCleanUp())
	})
	s.IntegrationTestSpanstore(t)
}
