// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"text/template"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	chdepstore "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	chtracestore "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

var (
	_ io.Closer          = (*Factory)(nil)
	_ depstore.Factory   = (*Factory)(nil)
	_ tracestore.Factory = (*Factory)(nil)
	_ storage.Purger     = (*Factory)(nil)
)

type Factory struct {
	config Configuration
	telset telemetry.Settings
	conn   driver.Conn
}

func NewFactory(ctx context.Context, cfg Configuration, telset telemetry.Settings) (*Factory, error) {
	cfg.applyDefaults()
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
	err = conn.Ping(ctx)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("failed to ping ClickHouse: %w", err),
			conn.Close(),
		)
	}
	if f.config.Schema.Create {
		ttlSeconds := int64(f.config.Schema.TraceTTL / time.Second)
		templateData := map[string]any{"TTLSeconds": ttlSeconds}

		spansTableQuery, err := executeTemplate("spans_table", sql.CreateSpansTable, templateData)
		if err != nil {
			return nil, errors.Join(err, conn.Close())
		}
		traceIDTimestampsQuery, err := executeTemplate("trace_id_timestamps_table", sql.CreateTraceIDTimestampsTable, templateData)
		if err != nil {
			return nil, errors.Join(err, conn.Close())
		}

		schemas := []struct {
			name  string
			query string
		}{
			{"spans table", spansTableQuery},
			{"services table", sql.CreateServicesTable},
			{"services materialized view", sql.CreateServicesMaterializedView},
			{"operations table", sql.CreateOperationsTable},
			{"operations materialized view", sql.CreateOperationsMaterializedView},
			{"trace id timestamps table", traceIDTimestampsQuery},
			{"trace id timestamps materialized view", sql.CreateTraceIDTimestampsMaterializedView},
			{"attribute metadata table", sql.CreateAttributeMetadataTable},
			{"attribute metadata materialized view", sql.CreateAttributeMetadataMaterializedView},
		}

		for _, schema := range schemas {
			if err = conn.Exec(ctx, schema.query); err != nil {
				return nil, errors.Join(fmt.Errorf("failed to create %s: %w", schema.name, err), conn.Close())
			}
		}
	} else if f.config.Schema.TraceTTL > 0 {
		// Apply TTL to existing tables when schema creation is disabled but TTL is configured
		if err = f.applyTTL(ctx, conn); err != nil {
			return nil, errors.Join(err, conn.Close())
		}
	}
	f.conn = conn
	return f, nil
}

func (f *Factory) CreateTraceReader() (tracestore.Reader, error) {
	return chtracestore.NewReader(f.conn, chtracestore.ReaderConfig{
		DefaultSearchDepth: f.config.DefaultSearchDepth,
		MaxSearchDepth:     f.config.MaxSearchDepth,
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

// executeTemplate parses and executes a Go template with the provided data.
func executeTemplate(name, templateSQL string, data map[string]any) (string, error) {
	tmpl, err := template.New(name).Parse(templateSQL)
	if err != nil {
		return "", fmt.Errorf("failed to parse %s template: %w", name, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute %s template: %w", name, err)
	}
	return buf.String(), nil
}

// applyTTL applies TTL settings to existing trace tables.
// This is used when Schema.Create is false but TraceTTL is configured,
// allowing users to update TTL on existing installations.
func (f *Factory) applyTTL(ctx context.Context, conn driver.Conn) error {
	seconds := int64(f.config.Schema.TraceTTL / time.Second)
	tables := []struct {
		name     string
		template string
	}{
		{"spans", sql.AlterSpansTTLTemplate},
		{"trace_id_timestamps", sql.AlterTraceIDTimestampsTTLTemplate},
	}

	for _, table := range tables {
		query := fmt.Sprintf(table.template, seconds)
		if err := conn.Exec(ctx, query); err != nil {
			return fmt.Errorf("failed to apply TTL to %s: %w", table.name, err)
		}
	}
	return nil
}
