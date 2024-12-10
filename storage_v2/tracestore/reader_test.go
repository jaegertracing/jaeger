// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/storage/spanstore"
)

func TestToSpanStoreQueryParameters(t *testing.T) {
	now := time.Now()
	query := &TraceQueryParameters{
		ServiceName:   "service",
		OperationName: "operation",
		Tags:          map[string]string{"tag-a": "val-a"},
		StartTimeMin:  now,
		StartTimeMax:  now.Add(time.Minute),
		DurationMin:   time.Minute,
		DurationMax:   time.Hour,
		NumTraces:     10,
	}
	expected := &spanstore.TraceQueryParameters{
		ServiceName:   "service",
		OperationName: "operation",
		Tags:          map[string]string{"tag-a": "val-a"},
		StartTimeMin:  now,
		StartTimeMax:  now.Add(time.Minute),
		DurationMin:   time.Minute,
		DurationMax:   time.Hour,
		NumTraces:     10,
	}
	require.Equal(t, expected, query.ToSpanStoreQueryParameters())
}
