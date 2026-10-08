// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/basicauthextension"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"

	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
	ch "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

type ClickHouseStorageIntegration struct {
	StorageIntegration
	factory *ch.Factory
}

func (s *ClickHouseStorageIntegration) initialize(t *testing.T) {
	cfg := ch.DefaultConfiguration()
	cfg.Addresses = []string{"127.0.0.1:9000"}
	cfg.Database = "jaeger"
	cfg.CreateSchema = true
	// The compose file runs a single replica with an embedded Keeper, so this suite creates
	// the schema with the replicated engines, and requireReplicatedTables checks that it did.
	// The e2e suite creates the local engines through cmd/jaeger/config-clickhouse.yaml; in CI
	// the two suites run in separate containers, and against one shared server whichever
	// suite creates the tables first fixes their engine, which the check then reports.
	cfg.TableEngine = ch.TableEngine{
		Replicated: configoptional.Some(ch.ReplicatedEngine{
			KeeperPath:  "/clickhouse/tables/{shard}/{database}/{table}",
			ReplicaName: "{replica}",
		}),
	}
	cfg.Auth = ch.Authentication{
		Basic: configoptional.Some(basicauthextension.ClientAuthSettings{
			Username: "default",
			Password: "password",
		}),
	}
	f, err := ch.NewFactory(context.Background(), cfg, telemetry.NoopSettings())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })
	s.factory = f
	requireReplicatedTables(t, cfg)

	s.TraceReader, err = f.CreateTraceReader()
	require.NoError(t, err)
	s.TraceWriter, err = f.CreateTraceWriter()
	require.NoError(t, err)
	s.DependencyReader, err = f.CreateDependencyReader()
	require.NoError(t, err)
	s.DependencyWriter, err = f.CreateDependencyWriter()
	require.NoError(t, err)
}

// requireReplicatedTables asserts that every table the factory created carries a Replicated*
// engine. Every CREATE TABLE is IF NOT EXISTS, so without this check a schema rendered with
// the local engines would pass the suite unnoticed.
func requireReplicatedTables(t *testing.T, cfg ch.Configuration) {
	basic := cfg.Auth.Basic.Get()
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: cfg.Addresses,
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: basic.Username,
			Password: string(basic.Password),
		},
	})
	require.NoError(t, err)
	defer conn.Close()
	rows, err := conn.Query(context.Background(),
		"SELECT name, engine FROM system.tables WHERE database = ? AND engine != 'MaterializedView' ORDER BY name",
		cfg.Database)
	require.NoError(t, err)
	defer rows.Close()
	engines := map[string]string{}
	for rows.Next() {
		var name, engine string
		require.NoError(t, rows.Scan(&name, &engine))
		engines[name] = engine
	}
	require.NoError(t, rows.Err())
	require.Equal(t, map[string]string{
		"attribute_metadata":  "ReplicatedAggregatingMergeTree",
		"dependencies":        "ReplicatedMergeTree",
		"operations":          "ReplicatedAggregatingMergeTree",
		"services":            "ReplicatedAggregatingMergeTree",
		"spans":               "ReplicatedMergeTree",
		"trace_id_timestamps": "ReplicatedAggregatingMergeTree",
	}, engines)
}

func (s *ClickHouseStorageIntegration) cleanUp(t *testing.T) {
	require.NoError(t, s.factory.Purge(context.Background()))
}

func TestClickHouseStorage(t *testing.T) {
	SkipUnlessEnv(t, StorageClickHouse)
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	s := &ClickHouseStorageIntegration{
		StorageIntegration: StorageIntegration{
			Capabilities: capabilities.ClickHouse(),
		},
	}
	s.CleanUp = s.cleanUp
	s.initialize(t)
	s.RunAll(t)
}
