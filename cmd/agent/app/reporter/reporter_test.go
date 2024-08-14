// Copyright (c) 2019 The Jaeger Authors.
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

package reporter

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/agent/app/testutils"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

func TestMultiReporter(t *testing.T) {
	r1, r2 := testutils.NewInMemoryReporter(), testutils.NewInMemoryReporter()
	r := NewMultiReporter(r1, r2)
	e1 := r.EmitZipkinBatch(context.Background(), []*zipkincore.Span{
		{},
	})
	e2 := r.EmitBatch(context.Background(), &jaeger.Batch{
		Spans: []*jaeger.Span{
			{},
		},
	})
	require.NoError(t, e1)
	require.NoError(t, e2)
	assert.Len(t, r1.ZipkinSpans(), 1)
	assert.Len(t, r1.Spans(), 1)
	assert.Len(t, r2.ZipkinSpans(), 1)
	assert.Len(t, r2.Spans(), 1)
}

func TestMultiReporterErrors(t *testing.T) {
	errMsg := "doh!"
	err := errors.New(errMsg)
	r1, r2 := mockReporter{err: err}, mockReporter{err: err}
	r := NewMultiReporter(r1, r2)
	e1 := r.EmitZipkinBatch(context.Background(), []*zipkincore.Span{
		{},
	})
	e2 := r.EmitBatch(context.Background(), &jaeger.Batch{
		Spans: []*jaeger.Span{
			{},
		},
	})
	require.EqualError(t, e1, fmt.Sprintf("%s\n%s", errMsg, errMsg))
	require.EqualError(t, e2, fmt.Sprintf("%s\n%s", errMsg, errMsg))
}

type mockReporter struct {
	err error
}

func (r mockReporter) EmitZipkinBatch(_ context.Context, _ []*zipkincore.Span) error {
	return r.err
}

func (r mockReporter) EmitBatch(_ context.Context, _ *jaeger.Batch) error {
	return r.err
}
