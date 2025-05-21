// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"context"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/config"
	chTraceStore "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/wrapper"
)

type NewConnFn func(ctx context.Context, cfg config.Config) (client.Conn, error)

type Factory struct {
	cfg  config.Config
	conn client.Conn

	newConnFn NewConnFn
}

func NewFactory(cfg config.Config) *Factory {
	return &Factory{
		cfg:       cfg,
		newConnFn: wrapper.WrapOpen,
	}
}

func (f *Factory) CreateTraceReader() (tracestore.Reader, error) {
	return chTraceStore.NewTraceReader(f.conn), nil
}

func (f *Factory) CreateTraceWriter() (tracestore.Writer, error) {
	return chTraceStore.NewTraceWriter(f.conn), nil
}

// Start attempts to connect to the ClickHouse server and acquire a connections pool for writing traces
// and a connection for reading traces.
func (f *Factory) Start(ctx context.Context) error {
	var err error
	f.conn, err = f.newConnFn(ctx, f.cfg)
	if err != nil {
		return err
	}
	return nil
}

func (f *Factory) Close() error {
	if f.conn != nil {
		return f.conn.Close()
	}
	return nil
}
