// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configtls"

	casconfig "github.com/jaegertracing/jaeger/internal/storage/cassandra/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore"
	cassandrav1 "github.com/jaegertracing/jaeger/internal/storage/v1/cassandra"
	"github.com/jaegertracing/jaeger/internal/storage/v2/cassandra"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
	"github.com/jaegertracing/jaeger/internal/telemetry"
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
	s.CleanUp = func(t *testing.T) {
		require.NoError(t, s.cleanUp())
	}
	return s
}

func (s *CassandraStorageIntegration) cleanUp() error {
	return s.factory.Purge(context.Background())
}

func (s *CassandraStorageIntegration) initializeCassandra() error {
	username := os.Getenv("CASSANDRA_USERNAME")
	password := os.Getenv("CASSANDRA_PASSWORD")

	cfg := casconfig.Configuration{
		Schema: casconfig.Schema{
			Keyspace: "jaeger_v1_dc1",
		},
		Connection: casconfig.Connection{
			Servers: []string{"127.0.0.1"},
			Authenticator: casconfig.Authenticator{
				Basic: casconfig.BasicAuthenticator{
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

	defCfg := casconfig.DefaultConfiguration()
	cfg.ApplyDefaults(&defCfg)

	opts := cassandrav1.Options{
		Configuration: cfg,
		Index: cassandrav1.IndexConfig{
			Logs:        true,
			Tags:        true,
			ProcessTags: true,
		},
		SpanStoreWriteCacheTTL: time.Hour * 12,
		ArchiveEnabled:         false,
	}

	f, err := cassandra.NewFactory(opts, telemetry.NoopSettings())
	if err != nil {
		return err
	}

	s.factory = f

	if s.TraceWriter, err = f.CreateTraceWriter(); err != nil {
		return err
	}
	if s.TraceReader, err = f.CreateTraceReader(); err != nil {
		return err
	}
	if s.SamplingStore, err = f.CreateSamplingStore(0); err != nil {
		return err
	}

	return s.initializeDependencyReaderAndWriter(f)
}

func (s *CassandraStorageIntegration) initializeDependencyReaderAndWriter(f *cassandra.Factory) error {
	dependencyReader, err := f.CreateDependencyReader()
	if err != nil {
		return err
	}
	s.DependencyReader = dependencyReader

	if dependencyWriter, ok := dependencyReader.(dependencystore.Writer); ok {
		s.DependencyWriter = v1adapter.NewDependencyWriter(dependencyWriter)
	}

	return nil
}

func TestCassandraStorage(t *testing.T) {
	SkipUnlessEnv(t, "cassandra")

	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})

	s := newCassandraStorageIntegration()

	require.NoError(t, s.initializeCassandra())
	t.Cleanup(func() {
		require.NoError(t, s.factory.Close())
	})

	s.RunAll(t)
}
