// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2019 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configtls"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/internal/metrics"
	casConfig "github.com/jaegertracing/jaeger/internal/storage/cassandra/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore"
	cassandrav1 "github.com/jaegertracing/jaeger/internal/storage/v1/cassandra"
	"github.com/jaegertracing/jaeger/internal/storage/v2/cassandra"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
	"github.com/jaegertracing/jaeger/internal/testutils"
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

func (s *CassandraStorageIntegration) initializeCassandra(t *testing.T) {
	username := os.Getenv("CASSANDRA_USERNAME")
	password := os.Getenv("CASSANDRA_PASSWORD")
	cfg := casConfig.Configuration{
		Schema: casConfig.Schema{
			Keyspace: "jaeger_v1_dc1",
		},
		Connection: casConfig.Connection{
			Servers: []string{"127.0.0.1"},
			Authenticator: casConfig.Authenticator{
				Basic: casConfig.BasicAuthenticator{
					Username:              username,
					Password:              password,
					AllowedAuthenticators: []string{"org.apache.cassandra.auth.PasswordAuthenticator"},
				},
			},
			TLS: configtls.ClientConfig{
				Insecure: true,
			},
		},
	}
	defCfg := casConfig.DefaultConfiguration()
	cfg.ApplyDefaults(&defCfg)
	opts := cassandrav1.Options{
		NamespaceConfig: cassandrav1.NamespaceConfig{Configuration: cfg},
		Index: cassandrav1.IndexConfig{
			Logs:        true,
			Tags:        true,
			ProcessTags: true,
		},
		SpanStoreWriteCacheTTL: time.Hour * 12,
	}
	f, err := cassandra.NewFactory(opts, metrics.NullFactory, zaptest.NewLogger(t))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, f.Close())
	})
	s.factory = f
	s.TraceWriter, err = f.CreateTraceWriter()
	require.NoError(t, err)
	s.TraceReader, err = f.CreateTraceReader()
	require.NoError(t, err)
	s.SamplingStore, err = f.CreateSamplingStore(0)
	require.NoError(t, err)
	s.initializeDependencyReaderAndWriter(t, f)
}

func (s *CassandraStorageIntegration) initializeDependencyReaderAndWriter(t *testing.T, f *cassandra.Factory) {
	var err error
	dependencyReader, err := f.CreateDependencyReader()
	require.NoError(t, err)
	s.DependencyReader = dependencyReader

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
