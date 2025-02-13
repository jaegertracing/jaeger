// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"errors"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/pkg/clickhouse"
	"github.com/jaegertracing/jaeger/pkg/clickhouse/config"
)

type Factory struct {
	config *config.Configuration
	logger *zap.Logger

	client clickhouse.Client
}

func NewFactoryWithConfig(configuration *config.Configuration, logger *zap.Logger) (*Factory, error) {
	client, err := configuration.NewClient(logger)
	if err != nil {
		return nil, err
	}

	f := &Factory{
		config: configuration,
		logger: logger,
		client: client,
	}
	return f, nil
}

func (f Factory) CreateTraceWriter() (tracestore.Writer, error) {
	return NewTraceWriter(f.client, f.logger, "otel_traces")
}

func (f Factory) Close() error {
	var errs []error
	if f.client != nil {
		errs = append(errs, f.client.Close())
	}
	return errors.Join(errs...)
}
