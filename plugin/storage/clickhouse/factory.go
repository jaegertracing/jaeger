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

	if err = chSpanStore.CreateSpansTable(ctx, f.client, f.spansTableName); err != nil {
		f.logger.Error("failed to create spans table", zap.Error(err))
	}

	return f

	// TODO: Move some steps to Initialize()
}

func (f *Factory) ChExportSpans(ctx context.Context, td ptrace.Traces) error {
	return chSpanStore.ExportSpans(ctx, f.client, f.spansTableName, td)
}

func (*Factory) Initialize(_ metrics.Factory, _ *zap.Logger) error {
	return nil
}

func (*Factory) CreateSpanReader() (spanstore.Reader, error) {
	return nil, nil
}

func (*Factory) CreateSpanWriter() (spanstore.Writer, error) {
	return nil, nil
}

func (*Factory) CreateDependencyReader() (dependencystore.Reader, error) {
	return nil, nil
}
