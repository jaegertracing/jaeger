// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"text/template"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore/metricstoremetrics"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	chdepstore "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/depstore"
	chmetricstore "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/metricstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	chtracestore "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

var (
	_ io.Closer                  = (*Factory)(nil)
	_ depstore.Factory           = (*Factory)(nil)
	_ tracestore.Factory         = (*Factory)(nil)
	_ storage.MetricStoreFactory = (*Factory)(nil)
	_ storage.Purger             = (*Factory)(nil)
)

// schemaTemplateParams is what the CREATE TABLE and materialized-view templates under
// sql/ are rendered with.
type schemaTemplateParams struct {
	// TTLSeconds is the retention of spans and trace_id_timestamps; zero renders no TTL clause.
	TTLSeconds int64
	// MergeTree is the ENGINE clause body of the tables that hold raw rows.
	MergeTree string
	// AggregatingMergeTree is the ENGINE clause body of the derived tables, whose rows
	// collapse on their sort key.
	AggregatingMergeTree string
	// HoistedColumns holds the column definitions of the hoisted attributes of the spans table.
	HoistedColumns []string
	// LocalTableSuffix is "_local" under the distributed layout (table_engine.cluster
	// set), "" otherwise. It shifts a table's own definition and every materialized view
	// around it onto the distributed layout together.
	LocalTableSuffix string
	// layout is the layout of the typed attribute groups of the spans table. The templates
	// reach it through the methods below.
	layout chtracestore.AttributeLayout
}

// ColumnTypeFn is the template function that renders the type of the column of an attribute
// group of the given type, such as bool.
func (p schemaTemplateParams) ColumnTypeFn(attributeType chtracestore.AttributeType) (string, error) {
	if !attributeType.Valid() {
		return "", fmt.Errorf("%q is not a type of attribute group", attributeType)
	}
	return p.layout.ColumnTypeSQL(attributeType), nil
}

// KeysFn is the template function that renders the keys of the attribute group of the given
// level and type, such as resource and bool.
func (p schemaTemplateParams) KeysFn(level expression.Level, attributeType chtracestore.AttributeType) (string, error) {
	group, err := chtracestore.NewAttributeGroup(level, attributeType)
	if err != nil {
		return "", err
	}
	return p.layout.KeysSQL(group), nil
}

// distributedTableParams is what create_distributed_table.sql is rendered with, once per
// table in the schema, under the distributed layout.
type distributedTableParams struct {
	// Table is the public name the Distributed table is created under; its local
	// counterpart is Table + "_local".
	Table string
	// Database is the configured database, threaded through because Distributed(...)
	// takes it as an argument rather than inferring it from the connection.
	Database string
	// Cluster is table_engine.cluster.name.
	Cluster string
	// ShardingKey is the expression Distributed(...) shards inserts by, or "" for a
	// table nothing inserts into directly, which needs none.
	ShardingKey string
}

type schemaStatement struct {
	name  string
	query string
}

type schemaBuilder struct {
	statements []schemaStatement
}

// newSchemaBuilder renders the statements that create the schema, for the attribute layout,
// the hoisted columns, and the distributed-layout cluster (nil for none) that the factory
// resolved from cfg.
func newSchemaBuilder(
	cfg Configuration,
	layout chtracestore.AttributeLayout,
	hoistedColumns []chtracestore.HoistedColumn,
	cluster *ClusterConfig,
) (*schemaBuilder, error) {
	mergeTree, err := cfg.TableEngine.engineClause(mergeTreeFamily)
	if err != nil {
		return nil, err
	}
	aggregatingMergeTree, err := cfg.TableEngine.engineClause(aggregatingMergeTreeFamily)
	if err != nil {
		return nil, err
	}
	suffix := ""
	if cluster != nil {
		suffix = "_local"
	}
	params := schemaTemplateParams{
		layout:               layout,
		TTLSeconds:           int64(cfg.TTL / time.Second),
		MergeTree:            mergeTree,
		AggregatingMergeTree: aggregatingMergeTree,
		LocalTableSuffix:     suffix,
	}
	for _, column := range hoistedColumns {
		params.HoistedColumns = append(params.HoistedColumns, column.Definition())
	}
	// distributedTable, when set, is the public name this table's underlying table is
	// additionally wrapped under as a Distributed table; shardingKey is its
	// Distributed(...) sharding key expression, empty for none. Only tables carry one, a
	// materialized view's target table carries its own entry instead.
	schema := []struct {
		name             string
		query            string
		distributedTable string
		shardingKey      string
	}{
		{"spans table", sql.CreateSpansTable, "spans", "cityHash64(trace_id)"},
		{"services table", sql.CreateServicesTable, "services", ""},
		{"services materialized view", sql.CreateServicesMaterializedView, "", ""},
		{"operations table", sql.CreateOperationsTable, "operations", ""},
		{"operations materialized view", sql.CreateOperationsMaterializedView, "", ""},
		{"trace id timestamps table", sql.CreateTraceIDTimestampsTable, "trace_id_timestamps", "cityHash64(trace_id)"},
		{"trace id timestamps materialized view", sql.CreateTraceIDTimestampsMaterializedView, "", ""},
		{"attribute metadata table", sql.CreateAttributeMetadataTable, "attribute_metadata", ""},
		{"attribute metadata materialized view", sql.CreateAttributeMetadataMaterializedView, "", ""},
		{"event attribute metadata materialized view", sql.CreateEventAttributeMetadataMaterializedView, "", ""},
		{"link attribute metadata materialized view", sql.CreateLinkAttributeMetadataMaterializedView, "", ""},
		{"dependencies table", sql.CreateDependenciesTable, "dependencies", "cityHash64(timestamp)"},
	}
	builder := &schemaBuilder{}
	for _, s := range schema {
		query, err := loadTemplate(s.name, s.query, params)
		if err != nil {
			return nil, err
		}
		builder.statements = append(builder.statements, schemaStatement{name: s.name, query: query})
		if cluster != nil && s.distributedTable != "" {
			wrapper, err := loadTemplate(s.distributedTable+" distributed table", sql.CreateDistributedTable, distributedTableParams{
				Table:       s.distributedTable,
				Database:    cfg.Database,
				Cluster:     cluster.Name,
				ShardingKey: s.shardingKey,
			})
			if err != nil {
				return nil, err
			}
			builder.statements = append(builder.statements, schemaStatement{
				name:  s.distributedTable + " distributed table",
				query: wrapper,
			})
		}
	}
	return builder, nil
}

func (b *schemaBuilder) build(ctx context.Context, conn driver.Conn) error {
	for _, statement := range b.statements {
		if err := conn.Exec(ctx, statement.query); err != nil {
			return fmt.Errorf("failed to create %s: %w", statement.name, err)
		}
	}
	return nil
}

type Factory struct {
	config Configuration
	telset telemetry.Settings
	conn   driver.Conn
	// hoistedColumns holds the configured hoisted columns. It is set only after the startup
	// check passes, which is what lets the reader search them.
	hoistedColumns []chtracestore.HoistedColumn
	// attributeLayout is the configured layout of the typed attribute groups, set with
	// hoistedColumns.
	attributeLayout chtracestore.AttributeLayout
}

func NewFactory(ctx context.Context, cfg Configuration, telset telemetry.Settings) (*Factory, error) {
	layout, err := cfg.attributeLayout()
	if err != nil {
		return nil, err
	}
	hoistedColumns, err := newHoistedColumns(cfg.HoistedAttributes, layout)
	if err != nil {
		return nil, err
	}
	var builder *schemaBuilder
	if cfg.CreateSchema {
		cluster, err := cfg.TableEngine.clusterConfig(cfg.Database)
		if err != nil {
			return nil, err
		}
		if cluster != nil && cfg.TableEngine.MergeTree.HasValue() {
			telset.Logger.Warn("table_engine.cluster.distributed_tables is set with merge_tree: " +
				"each shard holds its data without a replica, and losing a node loses that shard's data")
		}
		builder, err = newSchemaBuilder(cfg, layout, hoistedColumns, cluster)
		if err != nil {
			return nil, err
		}
	}

	f := &Factory{
		config: cfg,
		telset: telset,
	}
	opts := &clickhouse.Options{
		Protocol: getProtocol(f.config.Protocol),
		Addr:     f.config.Addresses,
		Auth: clickhouse.Auth{
			Database: f.config.Database,
		},
		DialTimeout: f.config.DialTimeout,
	}
	basicAuth := f.config.Auth.Basic.Get()
	if basicAuth != nil {
		opts.Auth.Username = basicAuth.Username
		opts.Auth.Password = string(basicAuth.Password)
	}
	if tlsCfg := f.config.TLS.Get(); tlsCfg != nil {
		loaded, tlsErr := tlsCfg.LoadTLSConfig(ctx)
		if tlsErr != nil {
			return nil, fmt.Errorf("failed to load TLS configuration: %w", tlsErr)
		}
		opts.TLS = loaded
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

	// The layout is checked before the schema is created, because CREATE TABLE IF NOT EXISTS
	// leaves a table of the other layout as it is, and a view that is created over that table
	// fails with an error that does not name the layout. A table that does not exist passes.
	if err := checkAttributeLayout(ctx, conn, layout); err != nil {
		return nil, err
	}

	if builder != nil {
		if err := builder.build(ctx, conn); err != nil {
			return nil, err
		}
	}

	// The hoisted columns are checked whether or not the schema was just built, because
	// CREATE TABLE IF NOT EXISTS leaves an existing table as it is.
	if err := checkHoistedColumns(ctx, conn, hoistedColumns); err != nil {
		return nil, err
	}

	success = true
	f.conn = conn
	f.hoistedColumns = hoistedColumns
	f.attributeLayout = layout
	return f, nil
}

func (f *Factory) CreateTraceReader() (tracestore.Reader, error) {
	return chtracestore.NewReader(f.conn, f.attributeLayout, chtracestore.ReaderConfig{
		HoistedColumns:                f.hoistedColumns,
		DefaultSearchDepth:            f.config.DefaultSearchDepth,
		MaxSearchDepth:                f.config.MaxSearchDepth,
		AttributeMetadataCacheTTL:     f.config.AttributeMetadataCacheTTL,
		AttributeMetadataCacheMaxSize: f.config.AttributeMetadataCacheMaxSize,
	}), nil
}

func (f *Factory) CreateTraceWriter() (tracestore.Writer, error) {
	return chtracestore.NewWriter(f.conn, f.attributeLayout), nil
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
		{"spans", sql.TruncateSpans},
		{"services", sql.TruncateServices},
		{"operations", sql.TruncateOperations},
		{"trace_id_timestamps", sql.TruncateTraceIDTimestamps},
		{"attribute_metadata", sql.TruncateAttributeMetadata},
		{"dependencies", sql.TruncateDependencies},
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
