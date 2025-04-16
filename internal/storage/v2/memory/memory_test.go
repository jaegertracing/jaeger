// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	v1 "github.com/jaegertracing/jaeger/internal/storage/v1/memory"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

func TestNewStore_DefaultConfig(t *testing.T) {
	store := NewStore(v1.Configuration{})
	td := loadInputTraces(t, 1)
	err := store.WriteTraces(context.Background(), td)
	require.NoError(t, err)
	tenant := store.getTenant(tenancy.GetTenant(context.Background()))
	traceID1 := fromString(t, "00000000000000010000000000000000")
	traces, ok := tenant.traces[traceID1]
	require.True(t, ok)
	expected := loadOutputTraces(t, 1)
	testTraces(t, expected, traces)
	traceID2 := fromString(t, "00000000000000020000000000000000")
	traces2, ok := tenant.traces[traceID2]
	require.True(t, ok)
	expected2 := loadOutputTraces(t, 2)
	testTraces(t, expected2, traces2)
}

func TestWriteTraces_WriteTwoBatches(t *testing.T) {
	store := NewStore(v1.Configuration{})
	traceId := fromString(t, "00000000000000010000000000000000")
	td1 := ptrace.NewTraces()
	td1.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty().SetTraceID(traceId)
	err := store.WriteTraces(context.Background(), td1)
	require.NoError(t, err)
	td2 := ptrace.NewTraces()
	td2.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty().SetTraceID(traceId)
	err = store.WriteTraces(context.Background(), td2)
	require.NoError(t, err)
	tenant := store.getTenant(tenancy.GetTenant(context.Background()))
	assert.Equal(t, 2, tenant.traces[traceId].ResourceSpans().Len())
}

func TestWriteTraces_WriteTraceWithTwoResourceSpans(t *testing.T) {
	store := NewStore(v1.Configuration{})
	traceId := fromString(t, "00000000000000010000000000000000")
	td := ptrace.NewTraces()
	resourceSpans := td.ResourceSpans()
	scopeSpan1 := resourceSpans.AppendEmpty().ScopeSpans().AppendEmpty()
	scopeSpan1.Spans().AppendEmpty().SetTraceID(traceId)
	scopeSpan1.Spans().AppendEmpty().SetTraceID(traceId)
	scopeSpan2 := resourceSpans.AppendEmpty().ScopeSpans().AppendEmpty()
	scopeSpan2.Spans().AppendEmpty().SetTraceID(traceId)
	scopeSpan2.Spans().AppendEmpty().SetTraceID(traceId)
	err := store.WriteTraces(context.Background(), td)
	require.NoError(t, err)
	tenant := store.getTenant(tenancy.GetTenant(context.Background()))
	// All spans have same trace id, so output should be same as input (that is no reshuffling, effectively)
	assert.Equal(t, td, tenant.traces[traceId])
}

func TestNewStore_TracesLimit(t *testing.T) {
	maxTraces := 5
	store := NewStore(v1.Configuration{
		MaxTraces: maxTraces,
	})
	for i := 1; i < 10; i++ {
		traceID := fromString(t, fmt.Sprintf("000000000000000%d0000000000000000", i))
		traces := ptrace.NewTraces()
		traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty().SetTraceID(traceID)
		err := store.WriteTraces(context.Background(), traces)
		require.NoError(t, err)
	}
	assert.Len(t, store.getTenant(tenancy.GetTenant(context.Background())).traces, maxTraces)
	assert.Len(t, store.getTenant(tenancy.GetTenant(context.Background())).ids, maxTraces)
}

func fromString(t *testing.T, dbTraceId string) pcommon.TraceID {
	var traceId [16]byte
	traceBytes, err := hex.DecodeString(dbTraceId)
	require.NoError(t, err)
	copy(traceId[:], traceBytes)
	return traceId
}

func testTraces(t *testing.T, expectedTraces ptrace.Traces, actualTraces ptrace.Traces) {
	if !assert.Equal(t, expectedTraces, actualTraces) {
		marshaller := ptrace.JSONMarshaler{}
		actualTd, err := marshaller.MarshalTraces(actualTraces)
		require.NoError(t, err)
		writeActualData(t, "traces", actualTd)
	}
}

func writeActualData(t *testing.T, name string, data []byte) {
	var prettyJson bytes.Buffer
	err := json.Indent(&prettyJson, data, "", "  ")
	require.NoError(t, err)
	path := "fixtures/actual_" + name + ".json"
	err = os.WriteFile(path, prettyJson.Bytes(), 0o644)
	require.NoError(t, err)
	t.Log("Saved the actual " + name + " to " + path)
}

func loadInputTraces(t *testing.T, i int) ptrace.Traces {
	return loadTraces(t, fmt.Sprintf("fixtures/otel_traces_%02d.json", i))
}

func loadOutputTraces(t *testing.T, i int) ptrace.Traces {
	return loadTraces(t, fmt.Sprintf("fixtures/db_traces_%02d.json", i))
}

func loadTraces(t *testing.T, name string) ptrace.Traces {
	unmarshller := ptrace.JSONUnmarshaler{}
	data, err := os.ReadFile(name)
	require.NoError(t, err)
	td, err := unmarshller.UnmarshalTraces(data)
	require.NoError(t, err)
	return td
}
