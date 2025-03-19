// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
)

func TestToSpanStoreQueryParameters(t *testing.T) {
	now := time.Now()
	attributes := pcommon.NewMap()
	attributes.PutStr("tag-a", "val-a")

	query := &TraceQueryParams{
		ServiceName:   "service",
		OperationName: "operation",
		Attributes:    attributes,
		StartTimeMin:  now,
		StartTimeMax:  now.Add(time.Minute),
		DurationMin:   time.Minute,
		DurationMax:   time.Hour,
		SearchDepth:   10,
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
