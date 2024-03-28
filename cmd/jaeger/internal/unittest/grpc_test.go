// Copyright (c) 2024 The Jaeger Authors.
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

//go:build grpc_storage_integration
// +build grpc_storage_integration

package unittest

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	googleGRPC "google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc"
	"github.com/jaegertracing/jaeger/plugin/storage/integration"
)

type gRPCServer struct {
	errChan chan error
	server  *googleGRPC.Server
	wg      sync.WaitGroup
}

func newgRPCServer() (*gRPCServer, error) {
	return &gRPCServer{errChan: make(chan error, 1)}, nil
}

type GRPCStorageUnitTestSuite struct {
	integration.StorageIntegration
	logger  *zap.Logger
	flags   []string
	factory *grpc.Factory
	server  *gRPCServer
}

func (s *GRPCStorageUnitTestSuite) initialize() error {
	s.logger, _ = testutils.NewLogger()
	f := grpc.NewFactory()
	v, command := config.Viperize(f.AddFlags)
	err := command.ParseFlags(s.flags)
	if err != nil {
		return err
	}
	f.InitFromViper(v, zap.NewNop())
	if err := f.Initialize(metrics.NullFactory, s.logger); err != nil {
		return err
	}
	s.factory = f

	if s.SpanWriter, err = f.CreateSpanWriter(); err != nil {
		return err
	}
	if s.SpanReader, err = f.CreateSpanReader(); err != nil {
		return err
	}

	// TODO DependencyWriter is not implemented in grpc store

	s.Refresh = func() error { return nil }
	s.CleanUp = s.cleanUp
	return nil
}

func (s *GRPCStorageUnitTestSuite) cleanUp() error {
	if err := s.factory.Close(); err != nil {
		return err
	}
	return s.initialize()
}

func TestGRPCRemoteStorage(t *testing.T) {
	flags := []string{
		"--grpc-storage.server=localhost:17271",
		"--grpc-storage.tls.enabled=false",
	}
	server, err := newgRPCServer()
	require.NoError(t, err)

	s := &GRPCStorageUnitTestSuite{
		flags:  flags,
		server: server,
	}
	require.NoError(t, s.initialize())
	s.IntegrationTestAll(t)
}
