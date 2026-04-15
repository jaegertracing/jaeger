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
	"go.opentelemetry.io/collector/featuregate"

	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	chdepstore "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	chtracestore "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

var clickhouseStorageGate = featuregate.GlobalRegistry().MustRegister(
	"storage.clickhouse",
	featuregate.StageAlpha,
	featuregate.WithRegisterFromVersion("v2.18.0"),
	featuregate.WithRegisterDescription(
		"Enables ClickHouse as a storage backend."),
)

var (
	_ io.Closer          = (*Factory)(nil)
	_ depstore.Factory   = (*Factory)(nil)
	_ tracestore.Factory = (*Factory)(nil)
	_ storage.Purger     = (*Factory)(nil)
)

type schemaTemplateParams struct {
	TTLSeconds int64
}

type schemaStatement struct {
	name  string
	query string
}

type schemaBuilder struct {
	statements []schemaStatement
}

func newSchemaBuilder(cfg Configuration) (*schemaBuilder, error) {
	createSpansTableQuery, err := loadTemplate(
		"create_spans_table",
		sql.CreateSpansTable,
		schemaTemplateParams{TTLSeconds: int64(cfg.TTL / time.Second)},
	)
	if err != nil {
		return nil, err
	}

	createTraceIDTsTableQuery, err := loadTemplate(
		"create_trace_id_timestamps_table",
		sql.CreateTraceIDTimestampsTable,
		schemaTemplateParams{TTLSeconds: int64(cfg.TTL / time.Second)},
	)
	if err != nil {
		return nil, err
	}

	return &schemaBuilder{
		statements: []schemaStatement{
			{"spans table", createSpansTableQuery},
			{"services table", sql.CreateServicesTable},
			{"services materialized view", sql.CreateServicesMaterializedView},
			{"operations table", sql.CreateOperationsTable},
			{"operations materialized view", sql.CreateOperationsMaterializedView},
			{"trace id timestamps table", createTraceIDTsTableQuery},
			{"trace id timestamps materialized view", sql.CreateTraceIDTimestampsMaterializedView},
			{"attribute metadata table", sql.CreateAttributeMetadataTable},
			{"attribute metadata materialized view", sql.CreateAttributeMetadataMaterializedView},
			{"event attribute metadata materialized view", sql.CreateEventAttributeMetadataMaterializedView},
			{"link attribute metadata materialized view", sql.CreateLinkAttributeMetadataMaterializedView},
			{"dependencies table", sql.CreateDependenciesTable},
		},
	}, nil
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
}

func NewFactory(ctx context.Context, cfg Configuration, telset telemetry.Settings) (*Factory, error) {
	if !clickhouseStorageGate.IsEnabled() {
		return nil, errors.New(
			"ClickHouse storage is experimental and must be explicitly enabled. " +
				"The schema is subject to breaking changes. " +
				"Enable it with --feature-gates=storage.clickhouse",
		)
	}
	fmt.Fprint(os.Stderr, `
*******************************************************************************

⚠️  WARNING: ClickHouse Storage is Experimental

You are using the ClickHouse storage backend, which is currently experimental.
The schema is subject to breaking changes in future releases.

⚠️  WARNING: ClickHouse Storage is Experimental

*******************************************************************************
`)
	cfg.applyDefaults()

	var builder *schemaBuilder
	if cfg.CreateSchema {
		var err error
		builder, err = newSchemaBuilder(cfg)
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

	if builder != nil {
		if err := builder.build(ctx, conn); err != nil {
			return nil, err
		}
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

func (*Factory) CreateDependencyReader() (depstore.Reader, error) {
	return chdepstore.NewDependencyReader(), nil
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
