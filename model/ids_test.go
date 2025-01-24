// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model_test

import (
	"testing"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpanIDFromBytes(t *testing.T) {
	errTests := [][]byte{
		{0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 13, 0},
	}
	for _, data := range errTests {
		_, err := model.SpanIDFromBytes(data)
		require.Error(t, err)
		require.EqualError(t, err, "invalid length for SpanID")
	}

	spanID, err := model.SpanIDFromBytes([]byte{0, 0, 0, 0, 0, 0, 0, 13})
	require.NoError(t, err)
	assert.Equal(t, spanID, model.NewSpanID(13))
}

func TestTraceIDFromBytes(t *testing.T) {
	errTests := [][]byte{
		{0, 0, 0, 0, 0, 0, 0, 13, 0, 0, 0, 0, 0, 0, 0, 0, 13},
		{0, 0, 0, 0, 0, 0, 0, 13, 0, 0, 0, 0, 0, 0, 13},
		{0, 0, 0, 0, 0, 0, 13},
	}
	for _, data := range errTests {
		_, err := model.TraceIDFromBytes(data)
		require.Error(t, err)
		assert.Equal(t, "invalid length for TraceID", err.Error())
	}

	tests := []struct {
		data     []byte
		expected model.TraceID
	}{
		{data: []byte{0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 3}, expected: model.NewTraceID(2, 3)},
		{data: []byte{0, 0, 0, 0, 0, 0, 0, 2}, expected: model.NewTraceID(0, 2)},
	}
	for _, test := range tests {
		traceID, err := model.TraceIDFromBytes(test.data)
		require.NoError(t, err)
		assert.Equal(t, test.expected, traceID)
	}
}
