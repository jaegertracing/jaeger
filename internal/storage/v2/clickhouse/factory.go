// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	chtracestore "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

var (
	_ io.Closer          = (*Factory)(nil)
	_ tracestore.Factory = (*Factory)(nil)
)

type Factory struct {
	config Configuration
	telset telemetry.Settings
	conn   driver.Conn
}

func NewFactory(ctx context.Context, cfg Configuration, telset telemetry.Settings) (*Factory, error) {
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
		if err = conn.Exec(ctx, sql.CreateSchema); err != nil {
			return nil, fmt.Errorf("failed to create ClickHouse schema: %w", err)
		}
	}
	f.conn = conn
	return f, nil
}

func (f *Factory) CreateTraceReader() (tracestore.Reader, error) {
	return chtracestore.NewReader(f.conn), nil
}

func (f *Factory) CreateTraceWriter() (tracestore.Writer, error) {
	return chtracestore.NewWriter(f.conn), nil
}

func (f *Factory) Close() error {
	return f.conn.Close()
}

func getProtocol(protocol string) clickhouse.Protocol {
	if protocol == "http" {
		return clickhouse.HTTP
	}
	return clickhouse.Native
}
