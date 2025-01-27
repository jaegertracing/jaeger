// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package reporter

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/agent/app/testutils"
	"github.com/jaegertracing/jaeger-idl/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger-idl/thrift-gen/zipkincore"
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
