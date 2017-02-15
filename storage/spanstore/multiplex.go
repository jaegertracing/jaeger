// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package spanstore

import (
	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/pkg/multierror"
)

// MultiplexWriter is a span Writer that tries to save spans into several underlying span Writers
type MultiplexWriter struct {
	spanWriters []Writer
}

// NewMultiplexWriter creates a MultiplexWriter
func NewMultiplexWriter(spanWriters ...Writer) *MultiplexWriter {
	return &MultiplexWriter{
		spanWriters: spanWriters,
	}
}

// WriteSpan calls WriteSpan on each span writer. It will sum up failures, it is not transactional
func (c *MultiplexWriter) WriteSpan(span *model.Span) error {
	var errors []error
	for _, writer := range c.spanWriters {
		if err := writer.WriteSpan(span); err != nil {
			errors = append(errors, err)
		}
	}
	return multierror.Wrap(errors)
}
