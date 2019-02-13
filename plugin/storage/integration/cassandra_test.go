// Copyright (c) 2019 Uber Technologies, Inc.
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

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
)

var (
	errInitializeCassandraDependencyWriter = errors.New("failed to initialize cassandra dependency writer")
)

type CassandraStorageIntegration struct {
	StorageIntegration

	logger *zap.Logger
}

func (s *CassandraStorageIntegration) initializeCassandra() error {
	s.logger, _ = testutils.NewLogger()
	var (
		f = cassandra.NewFactory()
	)
	v, command := config.Viperize(f.AddFlags)
	command.ParseFlags([]string{
		"--cassandra.keyspace=jaeger_v1_dc1",
	})
	f.InitFromViper(v)
	if err := f.Initialize(metrics.NullFactory, s.logger); err != nil {
		return err
	}
	var err error
	var ok bool
	if s.SpanWriter, err = f.CreateSpanWriter(); err != nil {
		return err
	}
	if s.SpanReader, err = f.CreateSpanReader(); err != nil {
		return err
	}
	if s.DependencyReader, err = f.CreateDependencyReader(); err != nil {
		return err
	}
	// TODO: Update this when the factory interface has CreateDependencyWriter
	if s.DependencyWriter, ok = s.DependencyReader.(dependencystore.Writer); !ok {
		return errInitializeCassandraDependencyWriter
	}
	s.Refresh = func() error { return nil }
	s.CleanUp = func() error { return nil }
	return nil
}

func TestCassandraStorage(t *testing.T) {
	if os.Getenv("STORAGE") != "cassandra" {
		t.Skip("Integration test against Cassandra skipped; set STORAGE env var to cassandra to run this")
	}
	s := &CassandraStorageIntegration{}
	require.NoError(t, s.initializeCassandra())
	// TODO: Support all other tests.
	t.Run("GetDependencies", s.testGetDependencies)
}
