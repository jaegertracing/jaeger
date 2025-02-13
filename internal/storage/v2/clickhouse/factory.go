// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/client"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/config"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/schema"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/trace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/wrapper"
)

type Factory struct {
	config *config.Configuration
	logger *zap.Logger

	connection client.Conn
	chPool     client.Pool
}

func newCHConn(cfg *config.Configuration) (client.Conn, error) {
	conn, err := wrapper.WrapOpen(cfg.ConnConfig)
	return conn, err
}

func newCHPool(cfg *config.Configuration, logger *zap.Logger) (client.Pool, error) {
	chPool, err := wrapper.WrapDial(cfg.PoolConfig, logger)
	return chPool, err
}

func newClientPrerequisites(c *config.Configuration, logger *zap.Logger) error {
	if c.CreateSchema {
		return nil
	}

	chPool, err := newCHPool(c, logger)
	if err != nil {
		return err
	}

	return schema.CreateSchemaIfNotPresent(chPool)
}

func newFactory() *Factory {
	return &Factory{}
}

func NewFactory(cfg *config.Configuration, logger *zap.Logger) (*Factory, error) {
	var errs []error
	err := newClientPrerequisites(cfg, logger)
	if err != nil {
		errs = append(errs, err)
	}

	connection, err := newCHConn(cfg)
	if connection == nil {
		errs = append(errs, err)
	}
	chPool, err := newCHPool(cfg, logger)
	if chPool == nil {
		errs = append(errs, err)
	}
	if errs != nil {
		return nil, errors.Join(errs...)
	}

	f := &Factory{
		config:     cfg,
		logger:     logger,
		connection: connection,
		chPool:     chPool,
	}
	return f, nil
}

func (f *Factory) CreateTraceWriter() (tracestore.Writer, error) {
	return trace.NewTraceWriter(f.chPool, f.logger)
}

func (f *Factory) CreateTracReader() (tracestore.Reader, error) {
	return trace.NewTraceReader(f.connection)
}

func (f *Factory) Purge(ctx context.Context) error {
	err := f.connection.Exec(ctx, "truncate otel_traces")
	return err
}

func (f *Factory) Close() error {
	if f.connection != nil {
		if err := f.connection.Close(); err != nil {
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
