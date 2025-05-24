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
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	conventions "go.opentelemetry.io/collector/semconv/v1.16.0"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	v1 "github.com/jaegertracing/jaeger/internal/storage/v1/memory"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

func TestNewStore_DefaultConfig(t *testing.T) {
	store, err := NewStore(v1.Configuration{
		MaxTraces: 10,
	})
	require.NoError(t, err)
	td := loadInputTraces(t, 1)
	err = store.WriteTraces(context.Background(), td)
	require.NoError(t, err)
	traceID1 := fromString(t, "00000000000000010000000000000000")
	traceID2 := fromString(t, "00000000000000020000000000000000")
	getTracesIter := store.GetTraces(context.Background(), tracestore.GetTraceParams{TraceID: traceID1}, tracestore.GetTraceParams{TraceID: traceID2})
	var traces []ptrace.Traces
	for gottd, err := range getTracesIter {
		require.NoError(t, err)
		assert.Len(t, gottd, 1)
		traces = append(traces, gottd[0])
	}
	expected := loadOutputTraces(t, 1)
	testTraces(t, expected, traces[0])
	expected2 := loadOutputTraces(t, 2)
	testTraces(t, expected2, traces[1])
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
	findTracesParams := tracestore.TraceQueryParams{
		ServiceName:   "service-x",
		OperationName: "test-general-conversion-2",
		Attributes:    queryAttrs,
		SearchDepth:   5,
	}
	idsIter := store.FindTraceIDs(context.Background(), findTracesParams)
	i := 0
	for foundTraceIds, err := range idsIter {
		i++
		require.NoError(t, err)
		assert.Len(t, foundTraceIds, 1)
		assert.Equal(t, traceID1, foundTraceIds[0].TraceID)
	}
	assert.Equal(t, 1, i)
	gotIter := store.FindTraces(context.Background(), findTracesParams)
	i = 0
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
	queryAttrs.PutStr("peer.service", "service-y")
	queryAttrs.PutDouble("temperature", 72.5)
	queryAttrs.PutBool(errorAttribute, true)
	queryAttrs.PutStr("event-x", "event-y")
	queryAttrs.PutStr("scope.attributes.2", "attribute-y")
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
	}{
		{
			name: "wrong service-name",
			modifyQueryFxn: func(query *tracestore.TraceQueryParams) {
				query.ServiceName = wrongStringValue
			},
		},
		{
			name: "wrong tag",
			modifyQueryFxn: func(query *tracestore.TraceQueryParams) {
				attrs := pcommon.NewMap()
				attrs.PutStr(wrongStringValue, wrongStringValue)
				attrs.MoveTo(query.Attributes)
			},
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
				query.Attributes.PutStr(errorAttribute, wrongStringValue)
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
	store, err := NewStore(v1.Configuration{
		MaxTraces: 10,
	})
	require.NoError(t, err)
	td := loadInputTraces(t, 1)
	err = store.WriteTraces(context.Background(), td)
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := tracestore.TraceQueryParams{
				ServiceName:   "service-x",
				OperationName: "test-general-conversion-2",
				Attributes:    getQueryAttributes(),
				SearchDepth:   10,
			}
			tt.modifyQueryFxn(&query)
			gotIter := store.FindTraces(context.Background(), query)
			iterLength := 0
			for _, err := range gotIter {
				require.NoError(t, err)
				iterLength++
			}
			assert.Equal(t, 0, iterLength)
		})
	}
}

func TestFindTracesAttributesMatching(t *testing.T) {
	stringVal := "val"
	tests := []struct {
		name       string
		attributes func(td ptrace.Traces) pcommon.Map
	}{
		{
			name: "resource-attributes",
			attributes: func(td ptrace.Traces) pcommon.Map {
				return td.ResourceSpans().At(0).Resource().Attributes()
			},
		},
		{
			name: "scope-attributes",
			attributes: func(td ptrace.Traces) pcommon.Map {
				return td.ResourceSpans().At(0).ScopeSpans().At(0).Scope().Attributes()
			},
		},
		{
			name: "span-attributes",
			attributes: func(td ptrace.Traces) pcommon.Map {
				return td.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
			},
		},
		{
			name: "event-attributes",
			attributes: func(td ptrace.Traces) pcommon.Map {
				return td.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Events().AppendEmpty().Attributes()
			},
		},
	}
	store, err := NewStore(v1.Configuration{
		MaxTraces: 10,
	})
	require.NoError(t, err)
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			td := ptrace.NewTraces()
			td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty().SetTraceID(fromString(t, fmt.Sprintf("000000000000000%d0000000000000000", i+1)))
			attrs := tt.attributes(td)
			attrs.PutStr(tt.name, stringVal)
			err := store.WriteTraces(context.Background(), td)
			require.NoError(t, err)
			iter := store.FindTraces(context.Background(), tracestore.TraceQueryParams{
				Attributes:  attrs,
				SearchDepth: 10,
			})
			iterLength := 0
			for traces, err := range iter {
				require.NoError(t, err)
				iterLength++
				assert.Len(t, traces, 1)
				assert.Equal(t, traces[0], td)
			}
			assert.Equal(t, 1, iterLength)
		})
	}
}

func TestFindTraces_MaxTraces(t *testing.T) {
	store, err := NewStore(v1.Configuration{
		MaxTraces: 10,
	})
	require.NoError(t, err)
	for i := 1; i < 9; i++ {
		td := ptrace.NewTraces()
		span := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		span.SetTraceID(fromString(t, fmt.Sprintf("000000000000000%d0000000000000000", i)))
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
		span.Attributes().PutBool("key", true)
		err := store.WriteTraces(context.Background(), td)
		require.NoError(t, err)
	}
	attrs := pcommon.NewMap()
	attrs.PutBool("key", true)
	params := tracestore.TraceQueryParams{
		SearchDepth: 5,
		Attributes:  attrs,
	}
	gotIter := store.FindTraces(context.Background(), params)
	iterLength := 0
	for traces, err := range gotIter {
		require.NoError(t, err)
		assert.Len(t, traces, 1)
		iterLength++
	}
	assert.Equal(t, 5, iterLength)
	newIter := store.FindTraces(context.Background(), params)
	iterLength = 0
	for _, err := range newIter {
		require.NoError(t, err)
		iterLength++
		if iterLength > 3 {
			break
		}
	}
	assert.Equal(t, 4, iterLength)
}

func TestFindTraces_AttributesFoundInEvents(t *testing.T) {
	store, err := NewStore(v1.Configuration{
		MaxTraces: 10,
	})
	require.NoError(t, err)
	td := ptrace.NewTraces()
	span := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetTraceID(fromString(t, "00000000000000010000000000000000"))
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	span.Events().AppendEmpty().Attributes().PutBool("key", true)
	err = store.WriteTraces(context.Background(), td)
	require.NoError(t, err)
	queryAttributes := pcommon.NewMap()
	queryAttributes.PutBool("key", true)
	params := tracestore.TraceQueryParams{
		Attributes:  queryAttributes,
		SearchDepth: 10,
	}
	gotIter := store.FindTraces(context.Background(), params)
	iterLength := 0
	for traces, err := range gotIter {
		iterLength++
		require.NoError(t, err)
		assert.Len(t, traces, 1)
		assert.Equal(t, td, traces[0])
	}
	assert.Equal(t, 1, iterLength)
}

func TestFindTraces_ErrorStatusNotMatched(t *testing.T) {
	store, err := NewStore(v1.Configuration{
		MaxTraces: 10,
	})
	require.NoError(t, err)
	td := ptrace.NewTraces()
	span := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetTraceID(fromString(t, "00000000000000010000000000000000"))
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	span.Status().SetCode(ptrace.StatusCodeOk)
	err = store.WriteTraces(context.Background(), td)
	require.NoError(t, err)
	queryAttributes := pcommon.NewMap()
	queryAttributes.PutBool(errorAttribute, true)
	params := tracestore.TraceQueryParams{
		Attributes:  queryAttributes,
		SearchDepth: 10,
	}
	gotIter := store.FindTraces(context.Background(), params)
	iterLength := 0
	for _, err := range gotIter {
		require.NoError(t, err)
		iterLength++
	}
	assert.Equal(t, 0, iterLength)
}

func TestFindTraces_NegativeSearchDepthErr(t *testing.T) {
	testInvalidSearchDepth(t, func(store *Store, params tracestore.TraceQueryParams) {
		gotIter := store.FindTraces(context.Background(), params)
		iterLength := 0
		for traces, err := range gotIter {
			iterLength++
			require.ErrorContains(t, err, errInvalidSearchDepth.Error())
			assert.Nil(t, traces)
		}
		assert.Equal(t, 1, iterLength)
	})
}

func TestFindTraceIds_NegativeSearchDepth(t *testing.T) {
	testInvalidSearchDepth(t, func(store *Store, params tracestore.TraceQueryParams) {
		gotIter := store.FindTraceIDs(context.Background(), params)
		iterLength := 0
		for traces, err := range gotIter {
			iterLength++
			require.ErrorContains(t, err, errInvalidSearchDepth.Error())
			assert.Nil(t, traces)
		}
		assert.Equal(t, 1, iterLength)
	})
}

func testInvalidSearchDepth(t *testing.T, fxn func(store *Store, params tracestore.TraceQueryParams)) {
	tests := []struct {
		name        string
		searchDepth int
	}{
		{
			name:        "negative search depth",
			searchDepth: -1,
		},
		{
			name:        "zero search depth",
			searchDepth: 0,
		},
		{
			name:        "search depth greater than max traces",
			searchDepth: 11,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, err := NewStore(v1.Configuration{
				MaxTraces: 10,
			})
			require.NoError(t, err)
			params := tracestore.TraceQueryParams{
				SearchDepth: test.searchDepth,
			}
			fxn(store, params)
		})
	}
}

func TestFindTraces_StatusCode(t *testing.T) {
	store, err := NewStore(v1.Configuration{
		MaxTraces: 10,
	})
	traceId1 := fromString(t, "00000000000000010000000000000000")
	traceId2 := fromString(t, "00000000000000020000000000000000")
	require.NoError(t, err)
	td := ptrace.NewTraces()
	spans := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans()
	span1 := spans.AppendEmpty()
	span2 := spans.AppendEmpty()
	span1.SetTraceID(traceId1)
	span1.Status().SetCode(ptrace.StatusCodeOk)
	span2.SetTraceID(traceId2)
	span2.Status().SetCode(ptrace.StatusCodeError)
	err = store.WriteTraces(context.Background(), td)
	require.NoError(t, err)
	queryAttributes := pcommon.NewMap()
	queryAttributes.PutBool(errorAttribute, true)
	iter1 := store.FindTraces(context.Background(), tracestore.TraceQueryParams{
		Attributes:  queryAttributes,
		SearchDepth: 10,
	})
	iterLength := 0
	for traces, err := range iter1 {
		require.NoError(t, err)
		iterLength++
		assert.Len(t, traces, 1)
		assert.Equal(t, traceId2, traces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).TraceID())
	}
	assert.Equal(t, 1, iterLength)
	iterLength = 0
	queryAttributes.PutBool(errorAttribute, false)
	iter2 := store.FindTraces(context.Background(), tracestore.TraceQueryParams{
		Attributes:  queryAttributes,
		SearchDepth: 10,
	})
	for traces, err := range iter2 {
		require.NoError(t, err)
		iterLength++
		assert.Len(t, traces, 1)
		assert.Equal(t, traceId1, traces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).TraceID())
	}
	assert.Equal(t, 1, iterLength)
}

func TestGetOperationsWithKind(t *testing.T) {
	store, err := NewStore(v1.Configuration{
		MaxTraces: 10,
	})
	require.NoError(t, err)
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
	err = store.WriteTraces(context.Background(), td)
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

func TestGetTraces_IterBreak(t *testing.T) {
	store, err := NewStore(v1.Configuration{
		MaxTraces: 10,
	})
	require.NoError(t, err)
	td := loadInputTraces(t, 1)
	err = store.WriteTraces(context.Background(), td)
	require.NoError(t, err)
	traceID1 := fromString(t, "00000000000000010000000000000000")
	traceID2 := fromString(t, "00000000000000020000000000000000")
	iter := store.GetTraces(context.Background(), tracestore.GetTraceParams{TraceID: traceID1}, tracestore.GetTraceParams{TraceID: traceID2})
	expected := loadOutputTraces(t, 1)
	iterLength := 1
	for traces, err := range iter {
		require.NoError(t, err)
		assert.Len(t, traces, 1)
		assert.Equal(t, expected, traces[0])
		break
	}
	assert.Equal(t, 1, iterLength)
}

func TestWriteTraces_WriteTwoBatches(t *testing.T) {
	store, err := NewStore(v1.Configuration{
		MaxTraces: 10,
	})
	require.NoError(t, err)
	traceId := fromString(t, "00000000000000010000000000000000")
	td1 := ptrace.NewTraces()
	td1.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty().SetTraceID(traceId)
	err = store.WriteTraces(context.Background(), td1)
	require.NoError(t, err)
	td2 := ptrace.NewTraces()
	td2.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty().SetTraceID(traceId)
	err = store.WriteTraces(context.Background(), td2)
	require.NoError(t, err)
	tenant := store.getTenant(tenancy.GetTenant(context.Background()))
	require.NoError(t, err)
	traceIndex := tenant.ids[traceId]
	assert.Equal(t, 2, tenant.traces[traceIndex].trace.ResourceSpans().Len())
}

func TestWriteTraces_WriteTraceWithTwoResourceSpans(t *testing.T) {
	store, err := NewStore(v1.Configuration{
		MaxTraces: 10,
	})
	require.NoError(t, err)
	traceId := fromString(t, "00000000000000010000000000000000")
	td := ptrace.NewTraces()
	resourceSpans := td.ResourceSpans()
	scopeSpan1 := resourceSpans.AppendEmpty().ScopeSpans().AppendEmpty()
	scopeSpan1.Spans().AppendEmpty().SetTraceID(traceId)
	scopeSpan1.Spans().AppendEmpty().SetTraceID(traceId)
	scopeSpan2 := resourceSpans.AppendEmpty().ScopeSpans().AppendEmpty()
	scopeSpan2.Spans().AppendEmpty().SetTraceID(traceId)
	scopeSpan2.Spans().AppendEmpty().SetTraceID(traceId)
	err = store.WriteTraces(context.Background(), td)
	require.NoError(t, err)
	tenant := store.getTenant(tenancy.GetTenant(context.Background()))
	require.NoError(t, err)
	traceIndex := tenant.ids[traceId]
	// All spans have same trace id, so output should be same as input (that is no reshuffling, effectively)
	assert.Equal(t, td, tenant.traces[traceIndex].trace)
}

func TestNewStore_TracesLimit(t *testing.T) {
	maxTraces := 8
	store, err := NewStore(v1.Configuration{
		MaxTraces: maxTraces,
	})
	require.NoError(t, err)
	writeTenTraces(t, store)
	tenant := store.getTenant(tenancy.GetTenant(context.Background()))
	require.NoError(t, err)
	assert.Len(t, tenant.traces, maxTraces)
	assert.Len(t, tenant.ids, maxTraces)
}

func TestNewStore_ReverseChronologicalOrder(t *testing.T) {
	maxTraces := 8
	store, err := NewStore(v1.Configuration{
		MaxTraces: maxTraces,
	})
	require.NoError(t, err)
	writeTenTraces(t, store)
	iter := store.FindTraces(context.Background(), tracestore.TraceQueryParams{
		SearchDepth: 5,
		Attributes:  pcommon.NewMap(),
	})
	// This test whether the traces are fetched in Reverse Chronological Order
	iterLength := 0
	for traces, err := range iter {
		require.NoError(t, err)
		assert.Len(t, traces, 1)
		actualTraceId := traces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).TraceID()
		assert.Equal(t, fromString(t, fmt.Sprintf("000000000000000%d0000000000000000", 9-iterLength)), actualTraceId)
		iterLength++
	}
	assert.Equal(t, 5, iterLength)
}

func TestInvalidMaxTracesErr(t *testing.T) {
	store, err := NewStore(v1.Configuration{})
	require.ErrorContains(t, err, errInvalidMaxTraces.Error())
	assert.Nil(t, store)
}

func TestGetDependencies(t *testing.T) {
	store, err := NewStore(v1.Configuration{
		MaxTraces: 10,
	})
	require.NoError(t, err)
	traceId := fromString(t, "00000000000000010000000000000000")
	td := ptrace.NewTraces()
	resourceSpans := td.ResourceSpans().AppendEmpty()
	resourceSpans.Resource().Attributes().PutStr(conventions.AttributeServiceName, "service-x")
	span1StartTime := time.Now()
	span1 := resourceSpans.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span1.SetTraceID(traceId)
	span1.SetSpanID(spanIdFromString(t, "0000000000000001"))
	span1.SetParentSpanID(spanIdFromString(t, "0000000000000003"))
	span1.SetStartTimestamp(pcommon.NewTimestampFromTime(span1StartTime))
	span1.SetEndTimestamp(pcommon.NewTimestampFromTime(span1StartTime.Add(1 * time.Second)))
	span2 := resourceSpans.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span2.SetTraceID(traceId)
	span2.SetSpanID(spanIdFromString(t, "0000000000000002"))
	span2.SetParentSpanID(spanIdFromString(t, "0000000000000003"))
	span2.SetStartTimestamp(pcommon.NewTimestampFromTime(span1StartTime.Add(1 * time.Second)))
	span2.SetEndTimestamp(pcommon.NewTimestampFromTime(span1StartTime.Add(2 * time.Second)))
	newResourceSpan := td.ResourceSpans().AppendEmpty()
	newResourceSpan.Resource().Attributes().PutStr(conventions.AttributeServiceName, "service-y")
	span3 := newResourceSpan.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span3.SetTraceID(traceId)
	span3.SetSpanID(spanIdFromString(t, "0000000000000003"))
	span3.SetStartTimestamp(pcommon.NewTimestampFromTime(span1StartTime.Add(-2 * time.Second)))
	span3.SetEndTimestamp(pcommon.NewTimestampFromTime(span1StartTime.Add(3 * time.Second)))
	span3.SetParentSpanID(spanIdFromString(t, "0000000000000004"))
	err = store.WriteTraces(context.Background(), td)
	require.NoError(t, err)
	deps, err := store.GetDependencies(context.Background(), depstore.QueryParameters{
		StartTime: span1StartTime.Add(-4 * time.Second),
		EndTime:   span1StartTime.Add(5 * time.Second),
	})
	require.NoError(t, err)
	assert.Len(t, deps, 1)
	assert.Equal(t, model.DependencyLink{
		Parent:    "service-y",
		Child:     "service-x",
		CallCount: 2,
	}, deps[0])
	td2 := ptrace.NewTraces()
	resourceSpan2 := td2.ResourceSpans().AppendEmpty()
	resourceSpan2.Resource().Attributes().PutStr(conventions.AttributeServiceName, "service-z")
	span4 := resourceSpan2.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span4.SetTraceID(traceId)
	span4.SetSpanID(spanIdFromString(t, "0000000000000004"))
	span4.SetStartTimestamp(pcommon.NewTimestampFromTime(span1StartTime.Add(-4 * time.Second)))
	span4.SetEndTimestamp(pcommon.NewTimestampFromTime(span1StartTime.Add(5 * time.Second)))
	err = store.WriteTraces(context.Background(), td2)
	require.NoError(t, err)
	newDeps, err := store.GetDependencies(context.Background(), depstore.QueryParameters{
		StartTime: span1StartTime.Add(-5 * time.Second),
		EndTime:   span1StartTime.Add(6 * time.Second),
	})
	require.NoError(t, err)
	assert.Len(t, newDeps, 2)
	newDeps2, err := store.GetDependencies(context.Background(), depstore.QueryParameters{
		StartTime: span1StartTime.Add(-5 * time.Second),
	})
	require.NoError(t, err)
	assert.Len(t, newDeps2, 2)
	emptyDeps, err := store.GetDependencies(context.Background(), depstore.QueryParameters{
		StartTime: span1StartTime.Add(-4 * time.Second),
		EndTime:   span1StartTime.Add(5 * time.Second),
	})
	require.NoError(t, err)
	assert.Empty(t, emptyDeps)
}

func TestGetDependencies_Err(t *testing.T) {
	store, err := NewStore(v1.Configuration{
		MaxTraces: 10,
	})
	require.NoError(t, err)
	startTime := time.Now()
	deps, err := store.GetDependencies(context.Background(), depstore.QueryParameters{
		StartTime: startTime,
		EndTime:   startTime.Add(-1 * time.Second),
	})
	require.ErrorContains(t, err, "end time must be greater than start time")
	assert.Nil(t, deps)
	deps, err = store.GetDependencies(context.Background(), depstore.QueryParameters{})
	require.ErrorContains(t, err, "start time is required")
	assert.Nil(t, deps)
}

func TestGetDependencies_EmptyParentSpanId(t *testing.T) {
	store, err := NewStore(v1.Configuration{
		MaxTraces: 10,
	})
	require.NoError(t, err)
	td := ptrace.NewTraces()
	span := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetTraceID(fromString(t, "00000000000000010000000000000000"))
	startTime := time.Now()
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(startTime))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(startTime.Add(1 * time.Second)))
	err = store.WriteTraces(context.Background(), td)
	require.NoError(t, err)
	deps, err := store.GetDependencies(context.Background(), depstore.QueryParameters{
		StartTime: startTime.Add(-1 * time.Second),
		EndTime:   startTime.Add(2 * time.Second),
	})
	require.NoError(t, err)
	assert.Empty(t, deps)
}

func TestGetDependencies_WrongSpanId(t *testing.T) {
	store, err := NewStore(v1.Configuration{
		MaxTraces: 10,
	})
	require.NoError(t, err)
	td := ptrace.NewTraces()
	span := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetTraceID(fromString(t, "00000000000000010000000000000000"))
	startTime := time.Now()
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(startTime))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(startTime.Add(1 * time.Second)))
	span.SetSpanID(spanIdFromString(t, "0000000000000002"))
	err = store.WriteTraces(context.Background(), td)
	require.NoError(t, err)
	deps, err := store.GetDependencies(context.Background(), depstore.QueryParameters{
		StartTime: startTime.Add(-1 * time.Second),
		EndTime:   startTime.Add(2 * time.Second),
	})
	require.NoError(t, err)
	assert.Empty(t, deps)
}

func writeTenTraces(t *testing.T, store *Store) {
	for i := 1; i < 10; i++ {
		traceID := fromString(t, fmt.Sprintf("000000000000000%d0000000000000000", i))
		traces := ptrace.NewTraces()
		traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty().SetTraceID(traceID)
		err := store.WriteTraces(context.Background(), traces)
		require.NoError(t, err)
	}
}

func fromString(t *testing.T, dbTraceId string) pcommon.TraceID {
	var traceId [16]byte
	traceBytes, err := hex.DecodeString(dbTraceId)
	require.NoError(t, err)
	copy(traceId[:], traceBytes)
	return traceId
}

func spanIdFromString(t *testing.T, dbTraceId string) pcommon.SpanID {
	var spanId [8]byte
	spanIdBytes, err := hex.DecodeString(dbTraceId)
	require.NoError(t, err)
	copy(spanId[:], spanIdBytes)
	return spanId
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
