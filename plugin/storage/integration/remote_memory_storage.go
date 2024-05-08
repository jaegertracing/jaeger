// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/cmd/remote-storage/app"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/healthcheck"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/plugin/storage"
	"github.com/jaegertracing/jaeger/ports"
)

type RemoteMemoryStorage struct {
	server         *app.Server
	storageFactory *storage.Factory
}

func StartNewRemoteMemoryStorage(t *testing.T) *RemoteMemoryStorage {
	logger := zaptest.NewLogger(t, zaptest.Level(zap.DebugLevel))
	opts := &app.Options{
		GRPCHostPort: ports.PortToHostPort(ports.RemoteStorageGRPC),
		Tenancy: tenancy.Options{
			Enabled: false,
		},
	}
	tm := tenancy.NewManager(&opts.Tenancy)
	storageFactory, err := storage.NewFactory(storage.FactoryConfigFromEnvAndCLI(os.Args, os.Stderr))
	require.NoError(t, err)

	v, _ := config.Viperize(storageFactory.AddFlags)
	storageFactory.InitFromViper(v, logger)
	require.NoError(t, storageFactory.Initialize(metrics.NullFactory, logger))

	t.Logf("Starting in-process remote storage server on %s", opts.GRPCHostPort)
	server, err := app.NewServer(opts, storageFactory, tm, logger, healthcheck.New())
	require.NoError(t, err)
	require.NoError(t, server.Start())

	return &RemoteMemoryStorage{
		server:         server,
		storageFactory: storageFactory,
	}
}

func (s *RemoteMemoryStorage) Close(t *testing.T) {
	require.NoError(t, s.server.Close())
	require.NoError(t, s.storageFactory.Close())
}
