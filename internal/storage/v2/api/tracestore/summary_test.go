// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"iter"
	"testing"

	"github.com/stretchr/testify/assert"
)

// summaryReader implements both Reader and SummaryReader.
type summaryReader struct {
	Reader
}

func (summaryReader) FindTraceSummaries(context.Context, TraceQueryParams) iter.Seq2[[]TraceSummary, error] {
	return func(func([]TraceSummary, error) bool) {}
}

// plainReader implements Reader but not SummaryReader.
type plainReader struct {
	Reader
}

// unwrapReader wraps a Reader and exposes Unwrap, like a metrics decorator.
type unwrapReader struct {
	Reader
	inner Reader
}

func (u unwrapReader) Unwrap() Reader { return u.inner }

func TestAsSummaryReader(t *testing.T) {
	native := summaryReader{}
	tests := []struct {
		name  string
		input Reader
		want  bool
	}{
		{"nil reader", nil, false},
		{"direct SummaryReader", native, true},
		{"plain reader without summary", plainReader{}, false},
		{"SummaryReader behind one Unwrap", unwrapReader{inner: native}, true},
		{"SummaryReader behind two Unwraps", unwrapReader{inner: unwrapReader{inner: native}}, true},
		{"no SummaryReader in unwrap chain", unwrapReader{inner: plainReader{}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AsSummaryReader(tt.input)
			if tt.want {
				assert.NotNil(t, got)
			} else {
				assert.Nil(t, got)
			}
		})
	}
}
