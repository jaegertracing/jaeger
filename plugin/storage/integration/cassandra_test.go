// Copyright (c) 2019 The Jaeger Authors.
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
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	dbsession "github.com/jaegertracing/jaeger/pkg/cassandra"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
)

type CassandraStorageIntegration struct {
	StorageIntegration

	session dbsession.Session
	logger  *zap.Logger
}

func newCassandraStorageIntegration() *CassandraStorageIntegration {
	s := &CassandraStorageIntegration{
		StorageIntegration: StorageIntegration{
			GetDependenciesReturnsSource: true,
			SkipArchiveTest:              true,

			SkipList: []string{
				"Tags_+_Operation_name_+_Duration_range",
				"Tags_+_Duration_range",
				"Tags_+_Operation_name_+_max_Duration",
				"Tags_+_max_Duration",
				"Operation_name_+_Duration_range",
				"Duration_range",
				"max_Duration",
				"Multiple_Traces",
			},
		},
	}
	s.CleanUp = s.cleanUp
	return s
}

func (s *CassandraStorageIntegration) cleanUp(t *testing.T) {
	require.NoError(t, s.session.Query("TRUNCATE traces").Exec())
}

func (s *CassandraStorageIntegration) initializeCassandraFactory(t *testing.T, flags []string) *cassandra.Factory {
	s.logger, _ = testutils.NewLogger()
	f := cassandra.NewFactory()
	v, command := config.Viperize(f.AddFlags)
	{
		err := command.ParseFlags(flags)
		require.NoError(t, err)
	}
	f.InitFromViper(v, zap.NewNop())
	{
		err := f.Initialize(metrics.NullFactory, s.logger)
		require.NoError(t, err)
	}
	return f
}

func (s *CassandraStorageIntegration) initializeCassandra(t *testing.T) {
	f := s.initializeCassandraFactory(t, []string{
		"--cassandra.keyspace=jaeger_v1_dc1",
	})
	s.session = f.PrimarySession()
	var err error
	s.SpanWriter, err = f.CreateSpanWriter()
	require.NoError(t, err)
	s.SpanReader, err = f.CreateSpanReader()
	require.NoError(t, err)
	s.SamplingStore, err = f.CreateSamplingStore(0)
	require.NoError(t, err)
	s.initializeDependencyReaderAndWriter(t, f)
	t.Cleanup(func() {
		require.NoError(t, f.Close())
	})
}

func (s *CassandraStorageIntegration) initializeDependencyReaderAndWriter(t *testing.T, f *cassandra.Factory) {
	var (
		err error
		ok  bool
	)
	s.DependencyReader, err = f.CreateDependencyReader()
	require.NoError(t, err)

	// TODO: Update this when the factory interface has CreateDependencyWriter
	if s.DependencyWriter, ok = s.DependencyReader.(dependencystore.Writer); !ok {
		t.Log("DependencyWriter not implemented ")
	}
}

func TestCassandraStorage(t *testing.T) {
	SkipUnlessEnv(t, "cassandra")
	s := newCassandraStorageIntegration()
	s.initializeCassandra(t)
	s.RunAll(t)
}
