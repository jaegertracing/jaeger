// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package factoryadapter

import (
	"context"
	"io"

	storage_v1 "github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage_v2/spanstore"
)

type Factory struct {
	ss storage_v1.Factory
}

func NewFactory(ss storage_v1.Factory) spanstore.Factory {
	return &Factory{
		ss: ss,
	}
}

// Initialize implements spanstore.Factory.
func (*Factory) Initialize(_ context.Context) error {
	panic("not implemented")
}

// Close implements spanstore.Factory.
func (f *Factory) Close(_ context.Context) error {
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
	return NewTraceReader(spanReader), nil
}

// CreateTraceWriter implements spanstore.Factory.
func (f *Factory) CreateTraceWriter() (spanstore.Writer, error) {
	spanWriter, err := f.ss.CreateSpanWriter()
	if err != nil {
		return nil, err
	}
	return NewTraceWriter(spanWriter), nil
}
