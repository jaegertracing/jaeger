// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package testutils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger-idl/thrift-gen/zipkincore"
)

func TestInMemoryReporter(t *testing.T) {
	r := NewInMemoryReporter()
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
	assert.Len(t, r.ZipkinSpans(), 1)
	assert.Len(t, r.Spans(), 1)
}
