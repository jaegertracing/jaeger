// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/jaegertracing/jaeger-idl/model/v1"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	conventions "go.opentelemetry.io/collector/semconv/v1.16.0"

	v1 "github.com/jaegertracing/jaeger/internal/storage/v1/memory"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
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
	operations, err := store.GetOperations(context.Background(), tracestore.OperationQueryParams{ServiceName: "service-x"})
	require.NoError(t, err)
	expectedOperations := []tracestore.Operation{
		{
			Name:     "test-general-conversion-2",
			SpanKind: "Server",
		},
		{
			Name:     "test-general-conversion-3",
			SpanKind: "Client",
		},
		{
			Name:     "test-general-conversion-4",
			SpanKind: "Producer",
		},
		{
			Name:     "test-general-conversion-5",
			SpanKind: "Consumer",
		},
	}
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].Name < operations[j].Name
	})
	assert.Equal(t, expectedOperations, operations)
	expectedServices := []string{"service-x"}
	services, err := store.GetServices(context.Background())
	require.NoError(t, err)
	assert.Equal(t, expectedServices, services)
	queryAttrs := getQueryAttributes()
	gotIter := store.FindTraces(context.Background(), tracestore.TraceQueryParams{
		ServiceName:   "service-x",
		OperationName: "test-general-conversion-2",
		Attributes:    queryAttrs,
	})
	i := 0
	for foundTraces, err := range gotIter {
		i++
		require.NoError(t, err)
		assert.Len(t, foundTraces, 1)
		testTraces(t, expected, foundTraces[0])
	}
	assert.Equal(t, 1, i)
}

func getQueryAttributes() pcommon.Map {
	queryAttrs := pcommon.NewMap()
	// These are overly constrained query attributes, typically they should match only one span
	queryAttrs.PutStr(conventions.OtelStatusCode, "Error")
	queryAttrs.PutStr(conventions.AttributeOtelScopeName, "testing-library-2")
	queryAttrs.PutStr(conventions.AttributeOtelScopeVersion, "1.1.1")
	queryAttrs.PutStr("peer.service", "service-y")
	queryAttrs.PutStr(model.SpanKindKey, "server")
	queryAttrs.PutDouble("temperature", 72.5)
	queryAttrs.PutStr(tagW3CTraceState, "state.2")
	return queryAttrs
}

func TestFindTraces_WrongQuery(t *testing.T) {
	wrongStringValue := "wrongStringValue"
	startTime := time.Unix(0, int64(1485467191639875000))
	endTime := time.Unix(0, int64(1485467191639880000))
	duration := endTime.Sub(startTime)
	tests := []struct {
		name           string
		modifyQueryFxn func(query *tracestore.TraceQueryParams)
		iterLength     int
	}{
		{
			name: "wrong service-name",
			modifyQueryFxn: func(query *tracestore.TraceQueryParams) {
				query.Attributes.PutStr(conventions.AttributeServiceName, wrongStringValue)
			},
		},
		{
			name: "wrong scope name",
			modifyQueryFxn: func(query *tracestore.TraceQueryParams) {
				query.Attributes.PutStr(conventions.AttributeOtelScopeName, wrongStringValue)
			},
		},
		{
			name: "wrong scope-version",
			modifyQueryFxn: func(query *tracestore.TraceQueryParams) {
				query.Attributes.PutStr(conventions.AttributeOtelScopeVersion, wrongStringValue)
			},
		},
		{
			name: "wrong tag",
			modifyQueryFxn: func(query *tracestore.TraceQueryParams) {
				query.Attributes.PutStr(wrongStringValue, wrongStringValue)
			},
		},
		{
			name: "wrong kind",
			modifyQueryFxn: func(query *tracestore.TraceQueryParams) {
				query.Attributes.PutStr(model.SpanKindKey, wrongStringValue)
			},
			iterLength: 1,
		},
		{
			name: "wrong operation name",
			modifyQueryFxn: func(query *tracestore.TraceQueryParams) {
				query.OperationName = wrongStringValue
			},
		},
		{
			name: "wrong status code",
			modifyQueryFxn: func(query *tracestore.TraceQueryParams) {
				query.Attributes.PutStr(conventions.AttributeOtelStatusCode, wrongStringValue)
			},
			iterLength: 1,
		},
		{
			name: "wrong status message",
			modifyQueryFxn: func(query *tracestore.TraceQueryParams) {
				query.Attributes.PutStr(conventions.AttributeOtelStatusDescription, wrongStringValue)
			},
		},
		{
			name: "wrong trace state",
			modifyQueryFxn: func(query *tracestore.TraceQueryParams) {
				query.Attributes.PutStr(tagW3CTraceState, wrongStringValue)
			},
		},
		{
			name: "wrong min start time",
			modifyQueryFxn: func(query *tracestore.TraceQueryParams) {
				query.StartTimeMin = startTime.Add(1 * time.Hour)
			},
		},
		{
			name: "wrong max start time",
			modifyQueryFxn: func(query *tracestore.TraceQueryParams) {
				query.StartTimeMax = startTime.Add(-1 * time.Hour)
			},
		},
		{
			name: "wrong min duration",
			modifyQueryFxn: func(query *tracestore.TraceQueryParams) {
				query.DurationMin = duration + 1*time.Hour
			},
		},
		{
			name: "wrong max duration",
			modifyQueryFxn: func(query *tracestore.TraceQueryParams) {
				query.DurationMax = duration - 1*time.Hour
			},
		},
	}
	store := NewStore(v1.Configuration{})
	td := loadInputTraces(t, 1)
	err := store.WriteTraces(context.Background(), td)
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := tracestore.TraceQueryParams{
				ServiceName:   "service-x",
				OperationName: "test-general-conversion-2",
				Attributes:    getQueryAttributes(),
			}
			tt.modifyQueryFxn(&query)
			gotIter := store.FindTraces(context.Background(), query)
			iterLength := 0
			for _, err := range gotIter {
				require.NoError(t, err)
				iterLength++
			}
			assert.Equal(t, tt.iterLength, iterLength)
		})
	}
}

func TestFindTraces_MaxTraces(t *testing.T) {

}

func TestGetOperationsWithKind(t *testing.T) {
	store := NewStore(v1.Configuration{})
	td := ptrace.NewTraces()
	resourceSpans := td.ResourceSpans().AppendEmpty()
	attrs := resourceSpans.Resource().Attributes()
	attrs.PutStr(conventions.AttributeServiceName, "service-x")
	span1 := resourceSpans.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span1.SetTraceID(fromString(t, "00000000000000010000000000000000"))
	span1.SetKind(ptrace.SpanKindConsumer)
	span1.SetName("operation-with-kind")
	span2 := resourceSpans.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span2.SetTraceID(fromString(t, "00000000000000010000000000000000"))
	err := store.WriteTraces(context.Background(), td)
	require.NoError(t, err)
	operations, err := store.GetOperations(context.Background(), tracestore.OperationQueryParams{
		ServiceName: "service-x",
		SpanKind:    ptrace.SpanKindConsumer.String(),
	})
	assert.Len(t, operations, 1)
	assert.Equal(t, "operation-with-kind", operations[0].Name)
	assert.Equal(t, operations[0].SpanKind, ptrace.SpanKindConsumer.String())
	require.NoError(t, err)
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
