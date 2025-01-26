// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

func TestTraceFindSpanByID(t *testing.T) {
	trace := &model.Trace{
		Spans: []*model.Span{
			{SpanID: model.NewSpanID(1), OperationName: "x"},
			{SpanID: model.NewSpanID(2), OperationName: "y"},
			{SpanID: model.NewSpanID(1), OperationName: "z"}, // same span ID
		},
	}
	s1 := trace.FindSpanByID(model.NewSpanID(1))
	assert.NotNil(t, s1)
	assert.Equal(t, "x", s1.OperationName)
	s2 := trace.FindSpanByID(model.NewSpanID(2))
	assert.NotNil(t, s2)
	assert.Equal(t, "y", s2.OperationName)
	s3 := trace.FindSpanByID(model.NewSpanID(3))
	assert.Nil(t, s3)
}

func TestTraceNormalizeTimestamps(t *testing.T) {
	s1 := "2017-01-26T16:46:31.639875-05:00"
	s2 := "2017-01-26T21:46:31.639875-04:00"
	var tt1, tt2 time.Time
	require.NoError(t, tt1.UnmarshalText([]byte(s1)))
	require.NoError(t, tt2.UnmarshalText([]byte(s2)))

	trace := &model.Trace{
		Spans: []*model.Span{
			{
				StartTime: tt1,
				Logs: []model.Log{
					{
						Timestamp: tt2,
					},
				},
			},
		},
	}
	trace.NormalizeTimestamps()
	span := trace.Spans[0]
	assert.Equal(t, span.StartTime, tt1.UTC())
	assert.Equal(t, span.Logs[0].Timestamp, tt2.UTC())
}
