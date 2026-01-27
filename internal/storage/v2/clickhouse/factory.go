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

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"go.uber.org/zap"

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
	if f.config.CreateSchema {
		schemas := []struct {
			name  string
			query string
		}{
			{"spans table", sql.CreateSpansTable},
			{"services table", sql.CreateServicesTable},
			{"services materialized view", sql.CreateServicesMaterializedView},
			{"operations table", sql.CreateOperationsTable},
			{"operations materialized view", sql.CreateOperationsMaterializedView},
			{"trace id timestamps table", sql.CreateTraceIDTimestampsTable},
			{"trace id timestamps materialized view", sql.CreateTraceIDTimestampsMaterializedView},
			{"attribute metadata table", sql.CreateAttributeMetadataTable},
			{"attribute metadata materialized view", sql.CreateAttributeMetadataMaterializedView},
		}

		templateData := struct {
			TTL int
		}{
			TTL: f.config.TTLDays(),
		}

		for _, schema := range schemas {
			rendered, err := renderTemplate(schema.query, templateData)
			if err != nil {
				return nil, errors.Join(fmt.Errorf("failed to render %s: %w", schema.name, err), conn.Close())
			}
			if err = conn.Exec(ctx, rendered); err != nil {
				return nil, errors.Join(fmt.Errorf("failed to create %s: %w", schema.name, err), conn.Close())
			}
		}

		if templateData.TTL > 0 {
			migrationQueries := []struct {
				table string
				col   string
			}{
				{"spans", "start_time"},
				{"trace_id_timestamps", "end"},
			}
			for _, m := range migrationQueries {
				query := fmt.Sprintf("ALTER TABLE %s MODIFY TTL %s + INTERVAL %d DAY", m.table, m.col, templateData.TTL)
				if err = conn.Exec(ctx, query); err != nil {
					f.telset.Logger.Warn("failed to migrate TTL", zap.String("table", m.table), zap.Error(err))
				}
			}
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

func renderTemplate(query string, data any) (string, error) {
	tmpl, err := template.New("sql").Parse(query)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
