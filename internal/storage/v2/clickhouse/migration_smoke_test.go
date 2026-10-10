// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"context"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/golang-migrate/migrate/v4"
	clickhousemigrate "github.com/golang-migrate/migrate/v4/database/clickhouse"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/basicauthextension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

func TestMigrationSmoke(t *testing.T) {
	cfg := DefaultConfiguration()
	cfg.Addresses = []string{"127.0.0.1:9000"}
	cfg.Database = "jaeger_smoke_test"
	cfg.CreateSchema = true
	cfg.Auth = Authentication{
		Basic: configoptional.Some(basicauthextension.ClientAuthSettings{
			Username: "default",
			Password: "password",
		}),
	}

	opts, err := Options(context.Background(), cfg)
	if err != nil {
		t.Skipf("skipping smoke test: %v", err)
	}
	db := clickhouse.OpenDB(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		t.Skipf("skipping smoke test: ClickHouse is not available at 127.0.0.1:9000 (%v)", err)
	}
	_ = db.Close()

	adminCfg := cfg
	adminCfg.Database = "default"
	adminOpts, err := Options(context.Background(), adminCfg)
	require.NoError(t, err)
	adminDB := clickhouse.OpenDB(adminOpts)
	defer adminDB.Close()

	testCtx := context.Background()
	_, err = adminDB.ExecContext(testCtx, "CREATE DATABASE IF NOT EXISTS jaeger_smoke_test")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = adminDB.ExecContext(context.Background(), "DROP DATABASE IF EXISTS jaeger_smoke_test")
	})

	smokeDB := clickhouse.OpenDB(opts)
	defer smokeDB.Close()

	dropAll := func() {
		for _, stmt := range DropSchemaObjectsStatements {
			_, err := smokeDB.ExecContext(testCtx, stmt)
			require.NoError(t, err)
		}
	}

	// 1. Clean up any existing objects
	dropAll()

	// 2. Apply v001 migration directly
	migrateDB := clickhouse.OpenDB(opts)
	defer migrateDB.Close()
	sourceDriver, err := iofs.New(sql.MigrationFiles, ".")
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
	f, err := NewFactory(testCtx, cfg, telemetry.NoopSettings())
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// 4. Dump schema DDL using SHOW CREATE TABLE
	migratedDDLs := make(map[string]string)
	for _, obj := range SchemaObjects {
		var ddl string
		err := smokeDB.QueryRowContext(testCtx, "SHOW CREATE TABLE "+obj).Scan(&ddl)
		require.NoError(t, err, "failed to SHOW CREATE TABLE for %s after migration", obj)
		migratedDDLs[obj] = ddl
	}

	// 5. Drop all objects
	dropAll()

	// 6. Recreate directly with the schema baseline
	baselineStmts, err := BaselineSchemaStatements()
	require.NoError(t, err)
	for _, stmt := range baselineStmts {
		_, err := smokeDB.ExecContext(testCtx, stmt)
		require.NoError(t, err, "failed to execute baseline statement: %s", stmt)
	}

	// 7. Dump schema DDL using SHOW CREATE TABLE and assert exact match
	for _, obj := range SchemaObjects {
		var baselineDDL string
		err := smokeDB.QueryRowContext(testCtx, "SHOW CREATE TABLE "+obj).Scan(&baselineDDL)
		require.NoError(t, err, "failed to SHOW CREATE TABLE for %s after baseline creation", obj)
		assert.Equal(t, baselineDDL, migratedDDLs[obj], "schema mismatch for object %s", obj)
	}
}
