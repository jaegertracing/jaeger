// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"context"
	"database/sql"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/metrics"
	chSpanStore "github.com/jaegertracing/jaeger/plugin/storage/clickhouse/spanstore"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

type Factory struct {
	client         *sql.DB
	spansTableName string
	logger         *zap.Logger
}

func NewFactory(ctx context.Context, cfg Config, logger *zap.Logger) *Factory {
	f := &Factory{
		logger:         logger,
		spansTableName: cfg.SpansTableName,
	}

	client, err := cfg.NewClient(ctx)
	if err != nil {
		f.logger.Error("failed to create ClickHouse client", zap.Error(err))
	} else {
		f.client = client
	}

	return f

	// TODO: Move some steps to Initialize()
}

func (f *Factory) CreateSpansTable(ctx context.Context) error {
	if err := chSpanStore.CreateSpansTable(ctx, f.client, f.spansTableName); err != nil {
		f.logger.Error("failed to create spans table", zap.Error(err))
	}
	return nil
}

func (f *Factory) ExportSpans(ctx context.Context, td ptrace.Traces) error {
	if err := chSpanStore.ExportSpans(ctx, f.client, f.spansTableName, td); err != nil {
		return err
	}

	return nil
}

func (f *Factory) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	return nil
}

func (f *Factory) CreateSpanReader() (spanstore.Reader, error) {
	return nil, nil
}

func (f *Factory) CreateSpanWriter() (spanstore.Writer, error) {
	return nil, nil
}

func (f *Factory) CreateDependencyReader() (dependencystore.Reader, error) {
	return nil, nil
}
