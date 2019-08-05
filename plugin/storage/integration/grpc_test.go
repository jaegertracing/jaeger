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

type GRPCStorageIntegrationTestSuite struct {
	StorageIntegration
	logger *zap.Logger
}

func (s *GRPCStorageIntegrationTestSuite) initialize() error {
	s.logger, _ = testutils.NewLogger()
	gopath := os.Getenv("GOPATH")
	path := gopath + "/src/github.com/jaegertracing/jaeger/examples/memstore-plugin/memstore-plugin"

	f := grpc.NewFactory()
	v, command := config.Viperize(f.AddFlags)
	err := command.ParseFlags([]string{
		"--grpc-storage-plugin.binary",
		path,
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
	s := &GRPCStorageIntegrationTestSuite{}
	require.NoError(t, s.initialize())
	s.IntegrationTestAll(t)
}
