// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"errors"

	"github.com/jaegertracing/jaeger/model"
)

// CompositeWriter is a span Writer that tries to save spans into several underlying span Writers
type CompositeWriter struct {
	spanWriters []Writer
}

// NewCompositeWriter creates a CompositeWriter
func NewCompositeWriter(spanWriters ...Writer) *CompositeWriter {
	return &CompositeWriter{
		spanWriters: spanWriters,
	}
}

// WriteSpan calls WriteSpan on each span writer. It will sum up failures, it is not transactional
func (c *CompositeWriter) WriteSpan(ctx context.Context, span *model.Span) error {
	var errs []error
	for _, writer := range c.spanWriters {
		if err := writer.WriteSpan(ctx, span); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
