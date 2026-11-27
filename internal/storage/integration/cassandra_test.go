// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2019 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configtls"

	casconfig "github.com/jaegertracing/jaeger/internal/storage/cassandra/config"
	caslock "github.com/jaegertracing/jaeger/internal/storage/distributedlock/cassandra"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
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
			Capabilities: capabilities.Cassandra(),
		},
	}
	s.CleanUp = s.cleanUp
	return s
}

func (s *CassandraStorageIntegration) cleanUp(t *testing.T) {
	require.NoError(t, s.factory.Purge(context.Background()))
}

func cassandraTestConfig() casconfig.Configuration {
	username := os.Getenv("CASSANDRA_USERNAME")
	password := os.Getenv("CASSANDRA_PASSWORD")
	cfg := casconfig.DefaultConfiguration()
	cfg.Schema.Keyspace = "jaeger_v1_dc1"
	cfg.Connection.Servers = []string{"127.0.0.1"}
	cfg.Connection.Authenticator.Basic = casconfig.BasicAuthenticator{
		Username:              username,
		Password:              password,
		AllowedAuthenticators: []string{"org.apache.cassandra.auth.PasswordAuthenticator"},
	}
	cfg.Connection.TLS = configtls.ClientConfig{Insecure: true}
	return cfg
}

func (s *CassandraStorageIntegration) initializeCassandra(t *testing.T) {
	opts := cassandrav1.Options{
		Configuration: cassandraTestConfig(),
		Index: cassandrav1.IndexConfig{
			Logs:        true,
			Tags:        true,
			ProcessTags: true,
		},
		SpanStoreWriteCacheTTL: time.Hour * 12,
		ArchiveEnabled:         false,
	}
	f, err := cassandra.NewFactory(opts, telemetry.NoopSettings())
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
	SkipUnlessEnv(t, StorageCassandra)
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	s := newCassandraStorageIntegration()
	s.initializeCassandra(t)
	s.RunAll(t)
}

func TestCassandraDistributedLock(t *testing.T) {
	SkipUnlessEnv(t, StorageCassandra)
	cfg := cassandraTestConfig()
	session, err := cassandrav1.NewSession(&cfg)
	require.NoError(t, err)
	t.Cleanup(session.Close)
	first := caslock.NewLock(session, "first")
	second := caslock.NewLock(session, "second")

	t.Run("forfeit", func(t *testing.T) {
		resource := uuid.NewString()
		acquired, err := first.Acquire(resource, time.Minute)
		require.NoError(t, err)
		require.True(t, acquired)

		_, err = second.Forfeit(resource)
		require.ErrorContains(t, err, "this host does not own the resource lock")
		acquired, err = second.Acquire(resource, time.Minute)
		require.NoError(t, err)
		require.False(t, acquired)

		forfeited, err := first.Forfeit(resource)
		require.NoError(t, err)
		require.True(t, forfeited)
		acquired, err = second.Acquire(resource, time.Minute)
		require.NoError(t, err)
		require.True(t, acquired)
	})

	t.Run("renewal", func(t *testing.T) {
		resource := uuid.NewString()
		acquired, err := first.Acquire(resource, time.Minute)
		require.NoError(t, err)
		require.True(t, acquired)

		acquired, err = first.Acquire(resource, 5*time.Minute)
		require.NoError(t, err)
		require.True(t, acquired)

		rows := session.Query("SELECT TTL(owner) FROM leases WHERE name = ?", resource).Iter()
		var ttlSeconds int
		found := rows.Scan(&ttlSeconds)
		require.NoError(t, rows.Close())
		require.True(t, found)
		require.Greater(t, ttlSeconds, 60)
		acquired, err = second.Acquire(resource, time.Minute)
		require.NoError(t, err)
		require.False(t, acquired)
	})

	t.Run("expiry", func(t *testing.T) {
		resource := uuid.NewString()
		acquired, err := first.Acquire(resource, 2*time.Second)
		require.NoError(t, err)
		require.True(t, acquired)

		require.EventuallyWithT(t, func(c *assert.CollectT) {
			acquired, err := second.Acquire(resource, time.Minute)
			assert.NoError(c, err)
			assert.True(c, acquired)
		}, 20*time.Second, 250*time.Millisecond)
	})
}
