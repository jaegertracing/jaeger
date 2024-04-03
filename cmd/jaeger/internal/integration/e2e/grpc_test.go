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
	"github.com/jaegertracing/jaeger/plugin/storage/integration"
	"github.com/jaegertracing/jaeger/ports"
)

type GRPCStorageIntegration struct {
	E2EStorageIntegration

	logger         *zap.Logger
	server         *app.Server
	storageFactory *storage.Factory
}

func (s *GRPCStorageIntegration) initialize(t *testing.T) {
	s.startServer(t)

	s.Refresh = func(_ *testing.T) {}
	s.CleanUp = s.cleanUp
}

func (s *GRPCStorageIntegration) startServer(t *testing.T) {
	opts := &app.Options{
		GRPCHostPort: ports.PortToHostPort(ports.RemoteStorageGRPC),
		Tenancy: tenancy.Options{
			Enabled: false,
		},
	}
	tm := tenancy.NewManager(&opts.Tenancy)
	var err error
	s.storageFactory, err = storage.NewFactory(storage.FactoryConfigFromEnvAndCLI(os.Args, os.Stderr))
	require.NoError(t, err)
	v, _ := config.Viperize(s.storageFactory.AddFlags)
	s.storageFactory.InitFromViper(v, s.logger)
	require.NoError(t, s.storageFactory.Initialize(metrics.NullFactory, s.logger))

	s.server, err = app.NewServer(opts, s.storageFactory, tm, s.logger, healthcheck.New())
	require.NoError(t, err)
	require.NoError(t, s.server.Start())
}

func (s *GRPCStorageIntegration) cleanUp(t *testing.T) {
	require.NoError(t, s.server.Close())
	require.NoError(t, s.storageFactory.Close())
	s.initialize(t)
}

func TestGRPCStorage(t *testing.T) {
	integration.SkipUnlessEnv(t, "grpc")

	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	s := &GRPCStorageIntegration{
		logger: logger,
	}
	s.ConfigFile = "../../../grpc_config.yaml"
	s.initialize(t)
	s.e2eInitialize(t)
	t.Cleanup(func() {
		s.e2eCleanUp(t)
	})
	s.RunTestSpanstore(t)
}
