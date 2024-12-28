// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc"
	"github.com/jaegertracing/jaeger/storage_v2/v1adapter"
)

type GRPCStorageIntegrationTestSuite struct {
	StorageIntegration
	flags         []string
	factory       *grpc.Factory
	remoteStorage *RemoteMemoryStorage
}

func (s *GRPCStorageIntegrationTestSuite) initialize(t *testing.T) {
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
	s.remoteStorage = StartNewRemoteMemoryStorage(t)

	f := grpc.NewFactory()
	v, command := config.Viperize(f.AddFlags)
	err := command.ParseFlags(s.flags)
	require.NoError(t, err)
	f.InitFromViper(v, logger)
	require.NoError(t, f.Initialize(metrics.NullFactory, logger))
	s.factory = f

	s.SpanWriter, err = f.CreateSpanWriter()
	require.NoError(t, err)
	spanReader, err := f.CreateSpanReader()
	require.NoError(t, err)
	s.TraceReader = v1adapter.NewTraceReader(spanReader)
	s.ArchiveSpanReader, err = f.CreateArchiveSpanReader()
	require.NoError(t, err)
	s.ArchiveSpanWriter, err = f.CreateArchiveSpanWriter()
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
		flags: []string{
			"--grpc-storage.server=localhost:17271",
			"--grpc-storage.tls.enabled=false",
		},
	}
	s.initialize(t)
	defer s.close(t)
	s.RunAll(t)
}
