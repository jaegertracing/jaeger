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
}

func (s *GRPCStorageIntegrationTestSuite) initialize(t *testing.T) {
	s.remoteStorage = StartNewRemoteMemoryStorage(t, ports.RemoteStorageGRPC)

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
	require.NoError(t, err)
	s.factory = f

	s.TraceWriter, err = f.CreateTraceWriter()
	require.NoError(t, err)
	s.TraceReader, err = f.CreateTraceReader()
	require.NoError(t, err)

	// TODO DependencyWriter is not implemented in grpc store

	s.CleanUp = s.cleanUp
}

func (s *GRPCStorageIntegrationTestSuite) close(t *testing.T) {
	require.NoError(t, s.factory.Close())
	s.remoteStorage.Close(t)
}

func (s *GRPCStorageIntegrationTestSuite) cleanUp(t *testing.T) {
	s.close(t)
	s.initialize(t)
}

func TestGRPCRemoteStorage(t *testing.T) {
	SkipUnlessEnv(t, "grpc")
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	s := &GRPCStorageIntegrationTestSuite{
		StorageIntegration: StorageIntegration{
			SkipList: GRPCSkippedTests,
		},
		flags: []string{
			"--grpc-storage.server=localhost:17271",
			"--grpc-storage.tls.enabled=false",
		},
	}
	s.initialize(t)
	defer s.close(t)
	s.RunAll(t)
}
