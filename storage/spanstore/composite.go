// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package spanstore

import (
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

// WriteSpan calls WriteSpan on each span writer. It will fail at the first error
func (c *CompositeWriter) WriteSpan(span *model.Span) error {
	for _, writer := range c.spanWriters {
		if err := writer.WriteSpan(span); err != nil {
			return err
		}
	}
	return nil
}
