// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/golang-migrate/migrate/v4"
	clickhousemigrate "github.com/golang-migrate/migrate/v4/database/clickhouse"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/basicauthextension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"

	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
	ch "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse"
	chsql "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
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

	s.TraceReader, err = f.CreateTraceReader()
	require.NoError(t, err)
	s.TraceWriter, err = f.CreateTraceWriter()
	require.NoError(t, err)
	s.DependencyReader, err = f.CreateDependencyReader()
	require.NoError(t, err)
	s.DependencyWriter, err = f.CreateDependencyWriter()
	require.NoError(t, err)
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

	cfg := ch.DefaultConfiguration()
	cfg.Addresses = []string{"127.0.0.1:9000"}
	cfg.Database = "jaeger_smoke_test"
	cfg.CreateSchema = true
	cfg.Auth = ch.Authentication{
		Basic: configoptional.Some(basicauthextension.ClientAuthSettings{
			Username: "default",
			Password: "password",
		}),
	}

	opts, err := ch.Options(context.Background(), cfg)
	require.NoError(t, err)

	adminCfg := cfg
	adminCfg.Database = "default"
	adminOpts, err := ch.Options(context.Background(), adminCfg)
	require.NoError(t, err)
	adminDB := clickhouse.OpenDB(adminOpts)
	defer adminDB.Close()

	ctx := context.Background()
	_, err = adminDB.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS jaeger_smoke_test")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = adminDB.ExecContext(context.Background(), "DROP DATABASE IF EXISTS jaeger_smoke_test")
	})

	smokeDB := clickhouse.OpenDB(opts)
	defer smokeDB.Close()

	dropAll := func() {
		for _, stmt := range ch.DropSchemaObjectsStatements {
			_, err := smokeDB.ExecContext(ctx, stmt)
			require.NoError(t, err)
		}
	}

	// 1. Clean up any existing objects
	dropAll()

	// 2. Apply v001 migration directly
	migrateDB := clickhouse.OpenDB(opts)
	defer migrateDB.Close()
	sourceDriver, err := iofs.New(chsql.MigrationFiles, ".")
	require.NoError(t, err)
	dbDriver, err := clickhousemigrate.WithInstance(migrateDB, &clickhousemigrate.Config{
		DatabaseName:          cfg.Database,
		MultiStatementEnabled: true,
	})
	require.NoError(t, err)
	m, err := migrate.NewWithInstance("iofs", sourceDriver, "clickhouse", dbDriver)
	require.NoError(t, err)
	err = m.Migrate(1)
	require.NoError(t, err)
	sourceErr, dbErr := m.Close()
	require.NoError(t, sourceErr)
	require.NoError(t, dbErr)

	// 3. Upgrade to latest version (v002) via NewFactory with CreateSchema: true
	f, err := ch.NewFactory(ctx, cfg, telemetry.NoopSettings())
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// 4. Dump schema DDL using SHOW CREATE TABLE
	migratedDDLs := make(map[string]string)
	for _, obj := range ch.SchemaObjects {
		var ddl string
		err := smokeDB.QueryRowContext(ctx, "SHOW CREATE TABLE "+obj).Scan(&ddl)
		require.NoError(t, err, "failed to SHOW CREATE TABLE for %s after migration", obj)
		migratedDDLs[obj] = ddl
	}

	// 5. Drop all objects
	dropAll()

	// 6. Recreate directly with the schema baseline
	baselineStmts, err := ch.BaselineSchemaStatements()
	require.NoError(t, err)
	for _, stmt := range baselineStmts {
		_, err := smokeDB.ExecContext(ctx, stmt)
		require.NoError(t, err, "failed to execute baseline statement: %s", stmt)
	}

	// 7. Dump schema DDL using SHOW CREATE TABLE and assert exact match
	for _, obj := range ch.SchemaObjects {
		var baselineDDL string
		err := smokeDB.QueryRowContext(ctx, "SHOW CREATE TABLE "+obj).Scan(&baselineDDL)
		require.NoError(t, err, "failed to SHOW CREATE TABLE for %s after baseline creation", obj)
		assert.Equal(t, baselineDDL, migratedDDLs[obj], "schema mismatch for object %s", obj)
	}
}
