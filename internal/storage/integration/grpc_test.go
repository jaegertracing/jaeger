// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/configtls"

	"github.com/jaegertracing/jaeger/internal/storage/v2/grpc"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/testutils"
	"github.com/jaegertracing/jaeger/ports"
)

type GRPCStorageIntegrationTestSuite struct {
	StorageIntegration
	flags         []string
	factory       *grpc.Factory
	remoteStorage *RemoteMemoryStorage
	t             *testing.T
}

func (s *GRPCStorageIntegrationTestSuite) initialize() error {
	remoteStorage, err := StartNewRemoteMemoryStorage(
		ports.RemoteStorageGRPC,
		nil, // no-op logger is fine for non-test context
	)
	if err != nil {
		return err
	}
	s.remoteStorage = remoteStorage

	f, err := grpc.NewFactory(
		context.Background(),
		grpc.Config{
			ClientConfig: configgrpc.ClientConfig{
				Endpoint: "localhost:17271",
				TLS: configtls.ClientConfig{
					Insecure: true,
				},
			},
		},
		telemetry.NoopSettings(),
	)
	if err != nil {
		return err
	}

	s.factory = f

	if s.TraceWriter, err = f.CreateTraceWriter(); err != nil {
		return err
	}
	if s.TraceReader, err = f.CreateTraceReader(); err != nil {
		return err
	}

	// TODO DependencyWriter is not implemented in grpc store
	return nil
}


func (s *GRPCStorageIntegrationTestSuite) close() error {
	if err := s.factory.Close(); err != nil {
		return err
	}
	s.remoteStorage.Close()
	return nil
}

func (s *GRPCStorageIntegrationTestSuite) cleanUp() error {
	if err := s.close(); err != nil {
		return err
	}
	return s.initialize()
}

func TestGRPCRemoteStorage(t *testing.T) {
	SkipUnlessEnv(t, "grpc")

	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})

	s := &GRPCStorageIntegrationTestSuite{
		flags: []string{
			"--grpc-storage.server=localhost:17271",
			"--grpc-storage.tls.enabled=false",
		},
		t: t,
	}

	require.NoError(t, s.initialize())
	t.Cleanup(func() {
		require.NoError(t, s.close())
	})

	s.CleanUp = func(t *testing.T) {
		require.NoError(t, s.cleanUp())
	}

	s.RunAll(t)
}
