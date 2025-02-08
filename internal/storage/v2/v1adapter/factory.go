// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	"context"
	"io"

	storage_v1 "github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

type Factory struct {
	ss storage_v1.Factory
}

func NewFactory(ss storage_v1.Factory) *Factory {
	return &Factory{
		ss: ss,
	}
}

// Initialize implements tracestore.Factory.
func (*Factory) Initialize(_ context.Context) error {
	panic("not implemented")
}

// Close implements tracestore.Factory.
func (f *Factory) Close(_ context.Context) error {
	if closer, ok := f.ss.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// CreateTraceReader implements tracestore.Factory.
func (f *Factory) CreateTraceReader() (tracestore.Reader, error) {
	spanReader, err := f.ss.CreateSpanReader()
	if err != nil {
		return nil, err
	}
	return NewTraceReader(spanReader), nil
}

// CreateTraceWriter implements tracestore.Factory.
func (f *Factory) CreateTraceWriter() (tracestore.Writer, error) {
	spanWriter, err := f.ss.CreateSpanWriter()
	if err != nil {
		return nil, err
	}
	return NewTraceWriter(spanWriter), nil
}

// CreateDependencyReader implements depstore.Factory.
func (f *Factory) CreateDependencyReader() (depstore.Reader, error) {
	dr, err := f.ss.CreateDependencyReader()
	if err != nil {
		return nil, err
	}
	return NewDependencyReader(dr), nil
}
