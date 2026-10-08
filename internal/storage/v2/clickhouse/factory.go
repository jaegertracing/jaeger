// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"text/template"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/cenkalti/backoff/v7"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	clickhousemigrate "github.com/golang-migrate/migrate/v4/database/clickhouse"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"go.opentelemetry.io/collector/featuregate"

	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore/metricstoremetrics"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	chdepstore "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/depstore"
	chmetricstore "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/metricstore"
	chsql "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	chtracestore "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

// ClickHouse is always available as a storage backend and is selected through
// configuration; no feature gate is required to enable it. The gate IDs below
// remain registered as Stable only so that existing --feature-gates values keep
// working (as no-ops) until they are removed in v2.23.0.
var (
	_ = featuregate.GlobalRegistry().MustRegister(
		"jaeger.clickhouse",
		featuregate.StageStable,
		featuregate.WithRegisterFromVersion("v2.21.0"),
		featuregate.WithRegisterToVersion("v2.23.0"),
		featuregate.WithRegisterDescription("No-op: ClickHouse is always available as a storage backend and is selected through configuration. Retained for backward compatibility and removed in v2.23.0."),
		featuregate.WithRegisterReferenceURL("https://github.com/jaegertracing/jaeger/issues/9016"),
	)
	_ = featuregate.GlobalRegistry().MustRegister(
		"storage.clickhouse",
		featuregate.StageStable,
		featuregate.WithRegisterFromVersion("v2.18.0"),
		featuregate.WithRegisterToVersion("v2.23.0"),
		featuregate.WithRegisterDescription("No-op: deprecated alias for jaeger.clickhouse, retained for backward compatibility and removed in v2.23.0."),
		featuregate.WithRegisterReferenceURL("https://github.com/jaegertracing/jaeger/issues/9016"),
	)
)

var (
	_ io.Closer                  = (*Factory)(nil)
	_ depstore.Factory           = (*Factory)(nil)
	_ tracestore.Factory         = (*Factory)(nil)
	_ storage.MetricStoreFactory = (*Factory)(nil)
	_ storage.Purger             = (*Factory)(nil)
)

type schemaTemplateParams struct {
	TTLSeconds int64
}

var (
	newSourceDriver    = iofs.New
	newDatabaseDriver  = clickhousemigrate.WithInstance
	newMigrateInstance = migrate.NewWithInstance
)

type schemaBuilder struct {
	cfg                    Configuration
	opts                   *clickhouse.Options
	backoffInitialInterval time.Duration
	backoffMaxElapsedTime  time.Duration
}

func newSchemaBuilder(cfg Configuration, opts *clickhouse.Options) *schemaBuilder {
	return &schemaBuilder{
		cfg:                    cfg,
		opts:                   opts,
		backoffInitialInterval: 100 * time.Millisecond,
		backoffMaxElapsedTime:  15 * time.Second,
	}
}

type drainingDatabaseDriver struct {
	database.Driver
}

func (d *drainingDatabaseDriver) Run(r io.Reader) error {
	err := d.Driver.Run(r)
	if err != nil {
		if closer, ok := r.(io.Closer); ok {
			_ = closer.Close()
		}
		_, _ = io.Copy(io.Discard, r)
	}
	return err
}

func latestBinaryVersion(src source.Driver) (uint, error) {
	v, err := src.First()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	for {
		next, err := src.Next(v)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return v, nil
			}
			return 0, err
		}
		v = next
	}
}

func (b *schemaBuilder) build(ctx context.Context) error {
	db := clickhouse.OpenDB(b.opts)
	defer db.Close()

	sourceDriver, err := newSourceDriver(chsql.MigrationFiles, ".")
	if err != nil {
		return fmt.Errorf("failed to create migration source driver: %w", err)
	}

	dbName := b.cfg.Database
	if dbName == "" {
		dbName = "default"
	}

	dbDriver, err := newDatabaseDriver(db, &clickhousemigrate.Config{
		DatabaseName:          dbName,
		MultiStatementEnabled: true,
	})
	if err != nil {
		_ = sourceDriver.Close()
		return fmt.Errorf("failed to create migration database driver: %w", err)
	}

	wrappedDriver := &drainingDatabaseDriver{Driver: dbDriver}
	m, err := newMigrateInstance("iofs", sourceDriver, "clickhouse", wrappedDriver)
	if err != nil {
		_ = sourceDriver.Close()
		_ = dbDriver.Close()
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer m.Close()

	binaryVersion, err := latestBinaryVersion(sourceDriver)
	if err != nil {
		return fmt.Errorf("failed to determine binary schema version: %w", err)
	}

	if b.cfg.CreateSchema {
		if err := b.applyMigrations(ctx, m); err != nil {
			return err
		}
	} else {
		dbVersion, _, err := m.Version()
		if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
			return fmt.Errorf("failed to read database schema version: %w", err)
		}
		if dbVersion > binaryVersion {
			return fmt.Errorf("database schema version %d is newer than binary version %d", dbVersion, binaryVersion)
		}
	}

	return nil
}

func (b *schemaBuilder) applyMigrations(ctx context.Context, m *migrate.Migrate) error {
	startTime := time.Now()
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = b.backoffInitialInterval
	expBackoff.MaxInterval = 2 * time.Second
	expBackoff.Reset()

	for {
		err := m.Up()
		if err == nil || errors.Is(err, migrate.ErrNoChange) {
			return nil
		}

		var dirtyErr migrate.ErrDirty
		if !errors.As(err, &dirtyErr) {
			return fmt.Errorf("failed to apply migrations: %w", err)
		}

		if time.Since(startTime) >= b.backoffMaxElapsedTime {
			return fmt.Errorf("schema migration dirty after backoff: %w", err)
		}

		delay := expBackoff.NextBackOff()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

// Options creates ClickHouse client options from the configuration.
func Options(ctx context.Context, cfg Configuration) (*clickhouse.Options, error) {
	opts := &clickhouse.Options{
		Protocol: getProtocol(cfg.Protocol),
		Addr:     cfg.Addresses,
		Auth: clickhouse.Auth{
			Database: cfg.Database,
		},
		DialTimeout: cfg.DialTimeout,
	}
	basicAuth := cfg.Auth.Basic.Get()
	if basicAuth != nil {
		opts.Auth.Username = basicAuth.Username
		opts.Auth.Password = string(basicAuth.Password)
	}
	if tlsCfg := cfg.TLS.Get(); tlsCfg != nil {
		loaded, tlsErr := tlsCfg.LoadTLSConfig(ctx)
		if tlsErr != nil {
			return nil, fmt.Errorf("failed to load TLS configuration: %w", tlsErr)
		}
		opts.TLS = loaded
	}
	return opts, nil
}

// SchemaObjects contains all tables and materialized views defined in the schema.
var SchemaObjects = []string{
	"spans",
	"services",
	"services_mv",
	"operations",
	"operations_mv",
	"trace_id_timestamps",
	"trace_id_timestamps_mv",
	"attribute_metadata",
	"attribute_metadata_mv",
	"event_attribute_metadata_mv",
	"link_attribute_metadata_mv",
	"dependencies",
}

// DropSchemaObjectsStatements contains DROP statements in safe dependency order.
var DropSchemaObjectsStatements = []string{
	"DROP VIEW IF EXISTS link_attribute_metadata_mv",
	"DROP VIEW IF EXISTS event_attribute_metadata_mv",
	"DROP VIEW IF EXISTS attribute_metadata_mv",
	"DROP VIEW IF EXISTS trace_id_timestamps_mv",
	"DROP VIEW IF EXISTS operations_mv",
	"DROP VIEW IF EXISTS services_mv",
	"DROP TABLE IF EXISTS dependencies",
	"DROP TABLE IF EXISTS attribute_metadata",
	"DROP TABLE IF EXISTS trace_id_timestamps",
	"DROP TABLE IF EXISTS operations",
	"DROP TABLE IF EXISTS services",
	"DROP TABLE IF EXISTS spans",
	"DROP TABLE IF EXISTS schema_migrations",
}

// BaselineSchemaStatements returns the 12 schema baseline DDL statements without TTL.
func BaselineSchemaStatements() ([]string, error) {
	createSpansTableQuery, err := loadTemplate(
		"create_spans_table",
		chsql.CreateSpansTable,
		schemaTemplateParams{TTLSeconds: 0},
	)
	if err != nil {
		return nil, err
	}

	createTraceIDTsTableQuery, err := loadTemplate(
		"create_trace_id_timestamps_table",
		chsql.CreateTraceIDTimestampsTable,
		schemaTemplateParams{TTLSeconds: 0},
	)
	if err != nil {
		return nil, err
	}

	return []string{
		createSpansTableQuery,
		chsql.CreateServicesTable,
		chsql.CreateServicesMaterializedView,
		chsql.CreateOperationsTable,
		chsql.CreateOperationsMaterializedView,
		createTraceIDTsTableQuery,
		chsql.CreateTraceIDTimestampsMaterializedView,
		chsql.CreateAttributeMetadataTable,
		chsql.CreateAttributeMetadataMaterializedView,
		chsql.CreateEventAttributeMetadataMaterializedView,
		chsql.CreateLinkAttributeMetadataMaterializedView,
		chsql.CreateDependenciesTable,
	}, nil
}

type Factory struct {
	config Configuration
	telset telemetry.Settings
	conn   driver.Conn
}

func NewFactory(ctx context.Context, cfg Configuration, telset telemetry.Settings) (*Factory, error) {
	opts, err := Options(ctx, cfg)
	if err != nil {
		return nil, err
	}

	builder := newSchemaBuilder(cfg, opts)

	f := &Factory{
		config: cfg,
		telset: telset,
	}

	conn, err := clickhouse.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create ClickHouse connection: %w", err)
	}

	success := false
	defer func() {
		if !success {
			_ = conn.Close()
		}
	}()

	if err = conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping ClickHouse: %w", err)
	}

	if err := builder.build(ctx); err != nil {
		return nil, err
	}

	success = true
	f.conn = conn
	return f, nil
}

func (f *Factory) CreateTraceReader() (tracestore.Reader, error) {
	return chtracestore.NewReader(f.conn, chtracestore.ReaderConfig{
		DefaultSearchDepth:            f.config.DefaultSearchDepth,
		MaxSearchDepth:                f.config.MaxSearchDepth,
		AttributeMetadataCacheTTL:     f.config.AttributeMetadataCacheTTL,
		AttributeMetadataCacheMaxSize: f.config.AttributeMetadataCacheMaxSize,
	}), nil
}

func (f *Factory) CreateTraceWriter() (tracestore.Writer, error) {
	return chtracestore.NewWriter(f.conn), nil
}

func (f *Factory) CreateDependencyReader() (depstore.Reader, error) {
	return chdepstore.NewDependencyReader(f.conn), nil
}

func (f *Factory) CreateDependencyWriter() (depstore.Writer, error) {
	return chdepstore.NewDependencyWriter(f.conn), nil
}

func (f *Factory) CreateMetricsReader() (metricstore.Reader, error) {
	return metricstoremetrics.NewReaderDecorator(
		chmetricstore.NewReader(f.conn),
		f.telset.Metrics,
	), nil
}

func (f *Factory) Close() error {
	return f.conn.Close()
}

func (f *Factory) Purge(ctx context.Context) error {
	tables := []struct {
		name  string
		query string
	}{
		{"spans", chsql.TruncateSpans},
		{"services", chsql.TruncateServices},
		{"operations", chsql.TruncateOperations},
		{"trace_id_timestamps", chsql.TruncateTraceIDTimestamps},
		{"attribute_metadata", chsql.TruncateAttributeMetadata},
		{"dependencies", chsql.TruncateDependencies},
	}

	for _, table := range tables {
		if err := f.conn.Exec(ctx, table.query); err != nil {
			return fmt.Errorf("failed to purge %s: %w", table.name, err)
		}
	}
	return nil
}

func getProtocol(protocol string) clickhouse.Protocol {
	if protocol == "http" {
		return clickhouse.HTTP
	}
	return clickhouse.Native
}

// loadTemplate is defined as a variable to allow overriding it in tests.
var loadTemplate func(name, tmplBody string, data any) (string, error) = loadTemplateImpl

func loadTemplateImpl(name, tmplBody string, data any) (string, error) {
	tmpl, err := template.New(name).Parse(tmplBody)
	if err != nil {
		return "", fmt.Errorf("failed to parse %s template: %w", name, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute %s template: %w", name, err)
	}
	return buf.String(), nil
}
