// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2019 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/v1adapter"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

type CassandraStorageIntegration struct {
	StorageIntegration
	factory *cassandra.Factory
}

func newCassandraStorageIntegration() *CassandraStorageIntegration {
	s := &CassandraStorageIntegration{
		StorageIntegration: StorageIntegration{
			GetDependenciesReturnsSource: true,

			SkipList: CassandraSkippedTests,
		},
	}
	s.CleanUp = s.cleanUp
	return s
}

func (s *CassandraStorageIntegration) cleanUp(t *testing.T) {
	require.NoError(t, s.factory.Purge(context.Background()))
}

func (*CassandraStorageIntegration) initializeCassandraFactory(t *testing.T, flags []string, factoryInit func() *cassandra.Factory) *cassandra.Factory {
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
	f := factoryInit()
	v, command := config.Viperize(f.AddFlags)
	require.NoError(t, command.ParseFlags(flags))
	f.InitFromViper(v, logger)
	require.NoError(t, f.Initialize(metrics.NullFactory, logger))
	t.Cleanup(func() {
		assert.NoError(t, f.Close())
	})
	return f
}

func (s *CassandraStorageIntegration) initializeCassandra(t *testing.T) {
	username := os.Getenv("CASSANDRA_USERNAME")
	password := os.Getenv("CASSANDRA_PASSWORD")
	f := s.initializeCassandraFactory(t, []string{
		"--cassandra.basic.allowed-authenticators=org.apache.cassandra.auth.PasswordAuthenticator",
		"--cassandra.password=" + password,
		"--cassandra.username=" + username,
		"--cassandra.keyspace=jaeger_v1_dc1",
	}, cassandra.NewFactory)
	s.factory = f
	var err error
	spanWriter, err := f.CreateSpanWriter()
	require.NoError(t, err)
	s.TraceWriter = v1adapter.NewTraceWriter(spanWriter)
	spanReader, err := f.CreateSpanReader()
	require.NoError(t, err)
	s.TraceReader = v1adapter.NewTraceReader(spanReader)
	s.SamplingStore, err = f.CreateSamplingStore(0)
	require.NoError(t, err)
	s.initializeDependencyReaderAndWriter(t, f)
}

func (s *CassandraStorageIntegration) initializeDependencyReaderAndWriter(t *testing.T, f *cassandra.Factory) {
	var err error
	dependencyReader, err := f.CreateDependencyReader()
	require.NoError(t, err)
	s.DependencyReader = v1adapter.NewDependencyReader(dependencyReader)

	// TODO: Update this when the factory interface has CreateDependencyWriter
	if dependencyWriter, ok := dependencyReader.(dependencystore.Writer); !ok {
		t.Log("DependencyWriter not implemented ")
	} else {
		s.DependencyWriter = v1adapter.NewDependencyWriter(dependencyWriter)
	}
}

func TestCassandraStorage(t *testing.T) {
	SkipUnlessEnv(t, "cassandra")
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	s := newCassandraStorageIntegration()
	s.initializeCassandra(t)
	s.RunAll(t)
}
