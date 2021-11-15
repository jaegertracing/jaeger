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
//go:build grpc_storage_integration
// +build grpc_storage_integration

package integration

import (
	"net"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
	googleGRPC "google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc"
	grpcMemory "github.com/jaegertracing/jaeger/plugin/storage/grpc/memory"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const defaultPluginBinaryPath = "../../../examples/memstore-plugin/memstore-plugin"

type gRPCServer struct {
	errChan chan error
	server  *googleGRPC.Server
	wg      sync.WaitGroup
}

func newgRPCServer() (*gRPCServer, error) {
	return &gRPCServer{errChan: make(chan error, 1)}, nil
}

func (s *gRPCServer) Restart() error {
	//stop the server if one already exists
	if s.server != nil {
		s.server.GracefulStop()
		s.wg.Wait()
		select {
		case err := <-s.errChan:
			return err
		default:
		}
	}

	memStorePlugin := grpcMemory.NewStoragePlugin(memory.NewStore(), memory.NewStore())

	s.server = googleGRPC.NewServer()
	queryPlugin := shared.StorageGRPCPlugin{
		Impl:        memStorePlugin,
		ArchiveImpl: memStorePlugin,
	}

	if err := queryPlugin.RegisterHandlers(s.server); err != nil {
		return err
	}

	listener, err := net.Listen("tcp", "localhost:2001")
	if err != nil {
		return err
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err = s.server.Serve(listener); err != nil {
			select {
			case s.errChan <- err:
			default:
			}
		}
	}()
	return nil
}

type GRPCStorageIntegrationTestSuite struct {
	StorageIntegration
	logger *zap.Logger
	flags  []string
	server *gRPCServer
}

func (s *GRPCStorageIntegrationTestSuite) initialize() error {
	s.logger, _ = testutils.NewLogger()

	if s.server != nil {
		if err := s.server.Restart(); err != nil {
			return err
		}
	}

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

	if s.SpanWriter, err = f.CreateSpanWriter(); err != nil {
		return err
	}
	if s.SpanReader, err = f.CreateSpanReader(); err != nil {
		return err
	}

	// TODO DependencyWriter is not implemented in grpc store

	s.Refresh = s.refresh
	s.CleanUp = s.cleanUp
	return nil
}

func (s *GRPCStorageIntegrationTestSuite) refresh() error {
	return nil
}

func (s *GRPCStorageIntegrationTestSuite) cleanUp() error {
	return s.initialize()
}

type memoryStorePlugin struct {
	store        *memory.Store
	archiveStore *memory.Store
}

func (ns *memoryStorePlugin) DependencyReader() dependencystore.Reader {
	return ns.store
}

func (ns *memoryStorePlugin) SpanReader() spanstore.Reader {
	return ns.store
}

func (ns *memoryStorePlugin) SpanWriter() spanstore.Writer {
	return ns.store
}

func (ns *memoryStorePlugin) ArchiveSpanReader() spanstore.Reader {
	return ns.archiveStore
}

func (ns *memoryStorePlugin) ArchiveSpanWriter() spanstore.Writer {
	return ns.archiveStore
}

func TestGRPCStorage(t *testing.T) {
	binaryPath := os.Getenv("PLUGIN_BINARY_PATH")
	if binaryPath == "" {
		t.Logf("PLUGIN_BINARY_PATH env var not set, using %s", defaultPluginBinaryPath)
		binaryPath = defaultPluginBinaryPath
	}
	configPath := os.Getenv("PLUGIN_CONFIG_PATH")
	if configPath == "" {
		t.Log("PLUGIN_CONFIG_PATH env var not set")
	}

	flags := []string{
		"--grpc-storage-plugin.binary", binaryPath,
		"--grpc-storage-plugin.log-level", "debug",
	}
	flags = append(flags,
		"--grpc-storage-plugin.configuration-file", configPath,
	)

	s := &GRPCStorageIntegrationTestSuite{
		flags: flags,
	}
	require.NoError(t, s.initialize())
	s.IntegrationTestAll(t)
}

func TestGRPCRemoteStorage(t *testing.T) {
	flags := []string{
		"--grpc-storage.server=localhost:2001",
		"--grpc-storage.tls.enabled=false",
	}
	server, err := newgRPCServer()
	require.NoError(t, err)

	s := &GRPCStorageIntegrationTestSuite{
		flags:  flags,
		server: server,
	}
	require.NoError(t, s.initialize())
	s.IntegrationTestAll(t)
}
