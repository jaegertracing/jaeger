// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package adjuster_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/model/adjuster"
)

func TestSequences(t *testing.T) {
	// mock adjuster that increments span ID
	adj := adjuster.Func(func(trace *model.Trace) {
		trace.Spans[0].SpanID++
	})

	seqAdjuster := adjuster.Sequence(adj, adj)

	span := &model.Span{}
	trace := model.Trace{Spans: []*model.Span{span}}

	seqAdjuster.Adjust(&trace)

	assert.Equal(t, span, trace.Spans[0], "same trace & span returned")
	assert.EqualValues(t, 2, span.SpanID, "expect span ID to be incremented")
}
