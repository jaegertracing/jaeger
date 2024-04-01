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
	"net"
	"os"
	"path"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	googleGRPC "google.golang.org/grpc"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc"
	grpcMemory "github.com/jaegertracing/jaeger/plugin/storage/grpc/memory"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
)

const (
	defaultPluginBinaryPath   = "../../../examples/memstore-plugin/memstore-plugin"
	streamingPluginConfigPath = "fixtures/grpc_plugin_conf.yaml"
)

type gRPCServer struct {
	errChan chan error
	server  *googleGRPC.Server
	wg      sync.WaitGroup
}

func newgRPCServer() (*gRPCServer, error) {
	return &gRPCServer{errChan: make(chan error, 1)}, nil
}

func (s *gRPCServer) Restart() error {
	// stop the server if one already exists
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
	logger  *zap.Logger
	flags   []string
	factory *grpc.Factory
	server  *gRPCServer
}

func (s *GRPCStorageIntegrationTestSuite) initialize(t *testing.T) {
	s.logger, _ = testutils.NewLogger()

	if s.server != nil {
		err := s.server.Restart()
		require.NoError(t, err)
	}

	f := grpc.NewFactory()
	v, command := config.Viperize(f.AddFlags)
	err := command.ParseFlags(s.flags)
	require.NoError(t, err)
	f.InitFromViper(v, zap.NewNop())
	err = f.Initialize(metrics.NullFactory, s.logger)
	require.NoError(t, err)
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

	s.Refresh = func(_ *testing.T) {}
	s.CleanUp = s.cleanUp
}

func (s *GRPCStorageIntegrationTestSuite) cleanUp(t *testing.T) {
	err := s.factory.Close()
	require.NoError(t, err)
	s.initialize(t)
}

func getPluginFlags(t *testing.T) []string {
	binaryPath := os.Getenv("PLUGIN_BINARY_PATH")
	if binaryPath == "" {
		t.Logf("PLUGIN_BINARY_PATH env var not set, using %s", defaultPluginBinaryPath)
		binaryPath = defaultPluginBinaryPath
	}

	return []string{
		"--grpc-storage-plugin.binary", binaryPath,
		"--grpc-storage-plugin.log-level", "debug",
	}
}

func TestGRPCStorage(t *testing.T) {
	skipUnlessEnv(t, "grpc")
	flags := getPluginFlags(t)
	if configPath := os.Getenv("PLUGIN_CONFIG_PATH"); configPath == "" {
		t.Log("PLUGIN_CONFIG_PATH env var not set")
	} else {
		flags = append(flags, "--grpc-storage-plugin.configuration-file", configPath)
	}

	s := &GRPCStorageIntegrationTestSuite{
		flags: flags,
	}
	s.initialize(t)
	s.RunAll(t)
}

func TestGRPCStreamingWriter(t *testing.T) {
	skipUnlessEnv(t, "grpc")
	flags := getPluginFlags(t)
	wd, err := os.Getwd()
	require.NoError(t, err)
	flags = append(flags,
		"--grpc-storage-plugin.configuration-file",
		path.Join(wd, streamingPluginConfigPath))

	s := &GRPCStorageIntegrationTestSuite{
		flags: flags,
	}
	s.initialize(t)
	s.RunAll(t)
}

func TestGRPCRemoteStorage(t *testing.T) {
	skipUnlessEnv(t, "grpc")
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
	s.initialize(t)
	s.RunAll(t)
}
