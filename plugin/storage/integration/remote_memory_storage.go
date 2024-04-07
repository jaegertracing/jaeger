// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"os"

	"go.uber.org/zap"

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

func StartNewRemoteMemoryStorage(logger *zap.Logger) (*RemoteMemoryStorage, error) {
	opts := &app.Options{
		GRPCHostPort: ports.PortToHostPort(ports.RemoteStorageGRPC),
		Tenancy: tenancy.Options{
			Enabled: false,
		},
	}
	tm := tenancy.NewManager(&opts.Tenancy)
	storageFactory, err := storage.NewFactory(storage.FactoryConfigFromEnvAndCLI(os.Args, os.Stderr))
	if err != nil {
		return nil, err
	}
	v, _ := config.Viperize(storageFactory.AddFlags)
	storageFactory.InitFromViper(v, logger)
	err = storageFactory.Initialize(metrics.NullFactory, logger)
	if err != nil {
		return nil, err
	}

	server, err := app.NewServer(opts, storageFactory, tm, logger, healthcheck.New())
	if err != nil {
		return nil, err
	}
	if err := server.Start(); err != nil {
		return nil, err
	}

	return &RemoteMemoryStorage{
		server:         server,
		storageFactory: storageFactory,
	}, nil
}

func (s *RemoteMemoryStorage) Close() error {
	if err := s.server.Close(); err != nil {
		return err
	}
	return s.storageFactory.Close()
}
