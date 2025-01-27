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
	"github.com/jaegertracing/jaeger/ports"
	"github.com/jaegertracing/jaeger/storage_v2/v1adapter"
)

type GRPCStorageIntegrationTestSuite struct {
	StorageIntegration
	flags                []string
	archiveFlags         []string
	factory              *grpc.Factory
	archiveFactory       *grpc.Factory
	remoteStorage        *RemoteMemoryStorage
	archiveRemoteStorage *RemoteMemoryStorage
}

func (s *GRPCStorageIntegrationTestSuite) initialize(t *testing.T) {
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
	s.remoteStorage = StartNewRemoteMemoryStorage(t, ports.RemoteStorageGRPC)
	s.archiveRemoteStorage = StartNewRemoteMemoryStorage(t, ports.RemoteStorageGRPC+1)

	initFactory := func(f *grpc.Factory, flags []string) {
		v, command := config.Viperize(f.AddFlags)
		require.NoError(t, command.ParseFlags(flags))
		f.InitFromViper(v, logger)
		require.NoError(t, f.Initialize(metrics.NullFactory, logger))
	}
	f := grpc.NewFactory()
	af := grpc.NewArchiveFactory()
	initFactory(f, s.flags)
	initFactory(af, s.archiveFlags)
	s.factory = f
	s.archiveFactory = af

	spanWriter, err := f.CreateSpanWriter()
	require.NoError(t, err)
	s.TraceWriter = v1adapter.NewTraceWriter(spanWriter)
	spanReader, err := f.CreateSpanReader()
	require.NoError(t, err)
	s.TraceReader = v1adapter.NewTraceReader(spanReader)

	// TODO DependencyWriter is not implemented in grpc store

	s.CleanUp = s.cleanUp
}

func (s *GRPCStorageIntegrationTestSuite) close(t *testing.T) {
	require.NoError(t, s.factory.Close())
	require.NoError(t, s.archiveFactory.Close())
	s.remoteStorage.Close(t)
	s.archiveRemoteStorage.Close(t)
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
		archiveFlags: []string{
			"--grpc-storage-archive.enabled=true",
			"--grpc-storage-archive.server=localhost:17272",
			"--grpc-storage-archive.tls.enabled=false",
		},
	}
	s.initialize(t)
	defer s.close(t)
	s.RunAll(t)
}
