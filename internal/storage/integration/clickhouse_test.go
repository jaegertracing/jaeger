// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"strings"
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
	requireStorageTuning(t, cfg)

	s.TraceReader, err = f.CreateTraceReader()
	require.NoError(t, err)
	s.TraceWriter, err = f.CreateTraceWriter()
	require.NoError(t, err)
	s.DependencyReader, err = f.CreateDependencyReader()
	require.NoError(t, err)
	s.DependencyWriter, err = f.CreateDependencyWriter()
	require.NoError(t, err)
}

func requireStorageTuning(t *testing.T, cfg ch.Configuration) {
	t.Helper()
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
	ctx := context.Background()
	rows, err := conn.Query(ctx,
		`SELECT table, name, type, compression_codec FROM system.columns
		WHERE database = ? AND table IN ('spans', 'services', 'operations', 'attribute_metadata', 'trace_id_timestamps', 'dependencies')
		ORDER BY table, position`,
		cfg.Database)
	require.NoError(t, err)
	defer rows.Close()
	dictionaryColumns := map[string]bool{
		"name": true, "kind": true, "status_code": true, "service_name": true,
		"scope_name": true, "scope_version": true, "events.name": true,
	}
	tables := map[string]bool{}
	for rows.Next() {
		var table, column, columnType, codec string
		require.NoError(t, rows.Scan(&table, &column, &columnType, &codec))
		tables[table] = true
		wantCodec := "CODEC(ZSTD(1))"
		if table == "spans" && column == "start_time" {
			wantCodec = "CODEC(Delta(8), ZSTD(1))"
		}
		require.Equal(t, wantCodec, codec, "%s.%s", table, column)
		switch table {
		case "spans":
			require.NotContains(t, columnType, "key String", column)
			if dictionaryColumns[column] || strings.HasSuffix(column, ".key") {
				require.Contains(t, columnType, "LowCardinality(String)", column)
			}
		case "services", "operations", "attribute_metadata":
			require.Equal(t, "LowCardinality(String)", columnType, "%s.%s", table, column)
		default:
			require.NotContains(t, columnType, "LowCardinality(String)", "%s.%s", table, column)
		}
	}
	require.NoError(t, rows.Err())
	require.Len(t, tables, 6)
	var query string
	require.NoError(t, conn.QueryRow(ctx, "SHOW CREATE TABLE spans").Scan(&query))
	require.Contains(t, query, "index_granularity = 8192")
	require.Contains(t, query, "ttl_only_drop_parts = 1")
	require.NoError(t, conn.QueryRow(ctx, "SHOW CREATE TABLE trace_id_timestamps").Scan(&query))
	require.NotContains(t, query, "ttl_only_drop_parts")
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
