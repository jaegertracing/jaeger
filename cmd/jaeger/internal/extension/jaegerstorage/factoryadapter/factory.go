// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package factoryadapter

import (
	"context"
	"io"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/metrics"
	storage_v1 "github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage_v2/spanstore"
)

type Factory struct {
	logger *zap.Logger
	ss     storage_v1.Factory
}

func NewFactory(logger *zap.Logger, ss storage_v1.Factory) spanstore.Factory {
	return &Factory{
		logger: logger,
		ss:     ss,
	}
}

// Initialize implements spanstore.Factory.
func (f *Factory) Initialize(ctx context.Context) error {
	return f.ss.Initialize(metrics.NullFactory, f.logger)
}

// Close implements spanstore.Factory.
func (f *Factory) Close(ctx context.Context) error {
	if closer, ok := f.ss.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// CreateTraceReader implements spanstore.Factory.
func (f *Factory) CreateTraceReader() (spanstore.Reader, error) {
	spanReader, err := f.ss.CreateSpanReader()
	if err != nil {
		return nil, err
	}
	return NewTraceReader(spanReader)
}

// CreateTraceWriter implements spanstore.Factory.
func (f *Factory) CreateTraceWriter() (spanstore.Writer, error) {
	spanWriter, err := f.ss.CreateSpanWriter()
	if err != nil {
		return nil, err
	}
	return NewTraceWriter(spanWriter), nil
}
