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

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage_v2/v1adapter"
)

type CassandraStorageIntegration struct {
	StorageIntegration
	factory        *cassandra.Factory
	archiveFactory *cassandra.Factory
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
	af := s.initializeCassandraFactory(t, []string{
		"--cassandra-archive.keyspace=jaeger_v1_dc1_archive",
		"--cassandra-archive.enabled=true",
		"--cassandra-archive.servers=127.0.0.1",
		"--cassandra-archive.basic.allowed-authenticators=org.apache.cassandra.auth.PasswordAuthenticator",
		"--cassandra-archive.password=" + password,
		"--cassandra-archive.username=" + username,
	}, cassandra.NewArchiveFactory)
	s.factory = f
	s.archiveFactory = af
	var err error
	spanWriter, err := f.CreateSpanWriter()
	require.NoError(t, err)
	s.TraceWriter = v1adapter.NewTraceWriter(spanWriter)
	spanReader, err := f.CreateSpanReader()
	require.NoError(t, err)
	s.TraceReader = v1adapter.NewTraceReader(spanReader)
	s.ArchiveSpanReader, err = af.CreateSpanReader()
	require.NoError(t, err)
	s.ArchiveSpanWriter, err = af.CreateSpanWriter()
	require.NoError(t, err)
	s.SamplingStore, err = f.CreateSamplingStore(0)
	require.NoError(t, err)
	s.initializeDependencyReaderAndWriter(t, f)
}

func (s *CassandraStorageIntegration) initializeDependencyReaderAndWriter(t *testing.T, f *cassandra.Factory) {
	var (
		err error
		ok  bool
	)
	dependencyReader, err := f.CreateDependencyReader()
	require.NoError(t, err)
	s.DependencyReader = v1adapter.NewDependencyReader(dependencyReader)

	// TODO: Update this when the factory interface has CreateDependencyWriter
	if s.DependencyWriter, ok = dependencyReader.(dependencystore.Writer); !ok {
		t.Log("DependencyWriter not implemented ")
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
