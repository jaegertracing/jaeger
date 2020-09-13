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
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc"
)

const defaultPluginBinaryPath = "../../../examples/memstore-plugin/memstore-plugin"

type GRPCStorageIntegrationTestSuite struct {
	StorageIntegration
	logger           *zap.Logger
	pluginBinaryPath string
}

func (s *GRPCStorageIntegrationTestSuite) initialize() error {
	s.logger, _ = testutils.NewLogger()

	f := grpc.NewFactory()
	v, command := config.Viperize(f.AddFlags)
	err := command.ParseFlags([]string{
		"--grpc-storage-plugin.binary",
		s.pluginBinaryPath,
	})
	if err != nil {
		return err
	}
	f.InitFromViper(v)
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

func TestGRPCStorage(t *testing.T) {
	if os.Getenv("STORAGE") != "grpc-plugin" {
		t.Skip("Integration test against grpc skipped; set STORAGE env var to grpc-plugin to run this")
	}
	path := os.Getenv("PLUGIN_BINARY_PATH")
	if path == "" {
		t.Logf("PLUGIN_BINARY_PATH env var not set, using %s", defaultPluginBinaryPath)
		path = defaultPluginBinaryPath
	}
	s := &GRPCStorageIntegrationTestSuite{
		pluginBinaryPath: path,
	}
	require.NoError(t, s.initialize())
	s.IntegrationTestAll(t)
}
