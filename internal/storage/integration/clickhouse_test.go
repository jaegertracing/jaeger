// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/basicauthextension"
	"github.com/stretchr/testify/assert"
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

func clickHouseIntegrationConfig() ch.Configuration {
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
	return cfg
}

func (s *ClickHouseStorageIntegration) initialize(t *testing.T) {
	cfg := clickHouseIntegrationConfig()
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
		"schema_migrations":   "ReplicatedMergeTree",
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

func TestClickHouseMigrationSmoke(t *testing.T) {
	SkipUnlessEnv(t, StorageClickHouse)
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})

	cfg := clickHouseIntegrationConfig()
	cfg.Database = "jaeger_smoke_test"
	cfg.TTL = 72 * time.Hour

	ctx := context.Background()
	adminCfg := cfg
	adminCfg.Database = "default"
	adminOpts, err := ch.Options(ctx, adminCfg)
	require.NoError(t, err)
	adminDB := clickhouse.OpenDB(adminOpts)
	defer adminDB.Close()

	_, err = adminDB.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS jaeger_smoke_test")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = adminDB.ExecContext(context.Background(), "DROP DATABASE IF EXISTS jaeger_smoke_test")
	})

	opts, err := ch.Options(ctx, cfg)
	require.NoError(t, err)
	smokeDB := clickhouse.OpenDB(opts)
	defer smokeDB.Close()

	dropAll := func() {
		rows, err := smokeDB.QueryContext(ctx, "SELECT name FROM system.tables WHERE database = 'jaeger_smoke_test' AND engine = 'MaterializedView'")
		require.NoError(t, err)
		var views []string
		for rows.Next() {
			var name string
			require.NoError(t, rows.Scan(&name))
			views = append(views, name)
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
		for _, v := range views {
			_, err := smokeDB.ExecContext(ctx, "DROP VIEW IF EXISTS jaeger_smoke_test."+v)
			require.NoError(t, err)
		}

		rows, err = smokeDB.QueryContext(ctx, "SELECT name FROM system.tables WHERE database = 'jaeger_smoke_test' AND engine != 'MaterializedView'")
		require.NoError(t, err)
		var tables []string
		for rows.Next() {
			var name string
			require.NoError(t, rows.Scan(&name))
			tables = append(tables, name)
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
		for _, tbl := range tables {
			_, err := smokeDB.ExecContext(ctx, "DROP TABLE IF EXISTS jaeger_smoke_test."+tbl)
			require.NoError(t, err)
		}
	}

	// 1. Clean up any existing objects dynamically
	dropAll()

	// 2. Apply v001 migration via NewFactory with CreateSchema: true
	f, err := ch.NewFactory(ctx, cfg, telemetry.NoopSettings())
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// 3. Retrieve created objects dynamically from system.tables
	rows, err := smokeDB.QueryContext(ctx, "SELECT name, engine FROM system.tables WHERE database = 'jaeger_smoke_test' ORDER BY name")
	require.NoError(t, err)
	defer rows.Close()
	found := make(map[string]string)
	for rows.Next() {
		var name, engine string
		require.NoError(t, rows.Scan(&name, &engine))
		found[name] = engine
	}
	require.NoError(t, rows.Err())

	expected := map[string]string{
		"attribute_metadata":          "ReplicatedAggregatingMergeTree",
		"attribute_metadata_mv":       "MaterializedView",
		"dependencies":                "ReplicatedMergeTree",
		"event_attribute_metadata_mv": "MaterializedView",
		"link_attribute_metadata_mv":  "MaterializedView",
		"operations":                  "ReplicatedAggregatingMergeTree",
		"operations_mv":               "MaterializedView",
		"schema_migrations":           "ReplicatedMergeTree",
		"services":                    "ReplicatedAggregatingMergeTree",
		"services_mv":                 "MaterializedView",
		"spans":                       "ReplicatedMergeTree",
		"trace_id_timestamps":         "ReplicatedAggregatingMergeTree",
		"trace_id_timestamps_mv":      "MaterializedView",
	}
	assert.Equal(t, expected, found)

	// 4. Verify TTL clauses were rendered in DDL
	var createSpansDDL string
	err = smokeDB.QueryRowContext(ctx, "SHOW CREATE TABLE jaeger_smoke_test.spans").Scan(&createSpansDDL)
	require.NoError(t, err)
	assert.Contains(t, createSpansDDL, "259200")

	var createTraceIDTsDDL string
	err = smokeDB.QueryRowContext(ctx, "SHOW CREATE TABLE jaeger_smoke_test.trace_id_timestamps").Scan(&createTraceIDTsDDL)
	require.NoError(t, err)
	assert.Contains(t, createTraceIDTsDDL, "259200")

	// 5. Verify migrations are idempotent
	f2, err := ch.NewFactory(ctx, cfg, telemetry.NoopSettings())
	require.NoError(t, err)
	require.NoError(t, f2.Close())

	// 6. Final cleanup
	dropAll()
}
