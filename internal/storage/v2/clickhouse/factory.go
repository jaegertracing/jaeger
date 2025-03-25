// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"context"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/config"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/schema"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/wrapper"
)

type Factory struct {
	config           *config.Configuration
	logger           *zap.Logger
	ClickhouseClient client.Clickhouse
	chPool           client.ChPool
}

func newClickhouseClient(cfg *config.Configuration) (client.Clickhouse, error) {
	clickhouse, err := wrapper.WrapOpen(cfg.ClickhouseConfig)
	return clickhouse, err
}

func newChPool(cfg *config.Configuration, logger *zap.Logger) (client.ChPool, error) {
	chPool, err := wrapper.WrapDial(cfg.ChPoolConfig, logger)
	return chPool, err
}

func newClientPrerequisites(cfg *config.Configuration, logger *zap.Logger) error {
	if !cfg.CreateSchema {
		return nil
	}

	chPool, err := newChPool(cfg, logger)
	if err != nil {
		return err
	}

	return schema.CreateSchemaIfNotPresent(chPool)
}

func newFactory() *Factory {
	return &Factory{}
}

func NewFactory(cfg *config.Configuration, logger *zap.Logger) (*Factory, error) {
	err := newClientPrerequisites(cfg, logger)
	if err != nil {
		return nil, err
	}

	clickhouse, err := newClickhouseClient(cfg)
	if err != nil {
		return nil, err
	}
	chPool, err := newChPool(cfg, logger)
	if err != nil {
		return nil, err
	}

	f := &Factory{
		config:           cfg,
		logger:           logger,
		ClickhouseClient: clickhouse,
		chPool:           chPool,
	}
	return f, nil
}

func (f *Factory) CreateTraceWriter() (*tracestore.TraceWriter, error) {
	return tracestore.NewTraceWriter(f.chPool, f.logger)
}

func (f *Factory) CreateTracReader() (*tracestore.TraceReader, error) {
	return tracestore.NewTraceReader(f.ClickhouseClient)
}

func (f *Factory) Purge(ctx context.Context) error {
	err := f.ClickhouseClient.Exec(ctx, "truncate otel_traces")
	return err
}

func (f *Factory) Close() error {
	if f.ClickhouseClient != nil {
		if err := f.ClickhouseClient.Close(); err != nil {
			return err
		}
	}
	if f.chPool != nil {
		if err := f.chPool.Close(); err != nil {
			return err
		}
	}
	return nil
}
