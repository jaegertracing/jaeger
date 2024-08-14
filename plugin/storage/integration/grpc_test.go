// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package integration

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc"
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
	s.SpanReader, err = f.CreateSpanReader()
	require.NoError(t, err)
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
