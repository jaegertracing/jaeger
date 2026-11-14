// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	conventions "github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

func TestWriteTraces_EmptyTraceIDRing(t *testing.T) {
	for _, tc := range []struct {
		name string
		last byte
		want []byte
	}{
		{name: "only empty"},
		{name: "before wrap", last: 4, want: []byte{1, 2, 3, 4}},
		{name: "after wrap", last: 8, want: []byte{4, 5, 6, 7, 8}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const capacity = 5
			store, err := NewStore(Configuration{MaxTraces: capacity})
			require.NoError(t, err)
			write := func(id byte) {
				td := ptrace.NewTraces()
				rs := td.ResourceSpans().AppendEmpty()
				service := "valid-service"
				if id == 0 {
					service = "discarded-service"
				}
				rs.Resource().Attributes().PutStr(conventions.ServiceNameKey, service)
				span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
				span.SetTraceID(pcommon.TraceID{id})
				span.SetSpanID(pcommon.SpanID{1})
				span.SetName("operation")
				require.NoError(t, store.WriteTraces(t.Context(), td))
			}
			// Separate writes make the ring order independent of map iteration.
			if tc.last > 0 {
				for id := byte(1); id <= 3; id++ {
					write(id)
				}
			}
			write(0)
			for id := byte(4); id <= tc.last; id++ {
				write(id)
			}

			var want []pcommon.TraceID
			for _, id := range tc.want {
				want = append(want, pcommon.TraceID{id})
			}
			tenant := store.getTenant("")
			assert.NotContains(t, tenant.ids, pcommon.TraceID{})
			assert.Len(t, tenant.ids, len(want))
			assert.Equal(t, (int(tc.last)-1)%capacity, tenant.mostRecent)
			for id, index := range tenant.ids {
				assert.Equal(t, id, tenant.traces[index].id)
			}

			query := tracestore.TraceQueryParams{SearchDepth: capacity, Attributes: pcommon.NewMap()}
			traces, err := jiter.FlattenWithErrors(store.FindTraces(t.Context(), query))
			require.NoError(t, err)
			var found []pcommon.TraceID
			for _, trace := range traces {
				found = append(found, jptrace.GetTraceID(trace))
			}
			assert.ElementsMatch(t, want, found)
			var foundIDs []pcommon.TraceID
			for chunk, err := range store.FindTraceIDs(t.Context(), query) {
				require.NoError(t, err)
				for _, trace := range chunk.Results {
					foundIDs = append(foundIDs, trace.TraceID)
				}
			}
			assert.ElementsMatch(t, want, foundIDs)
			zero, err := jiter.FlattenWithErrors(store.GetTraces(t.Context(), tracestore.GetTraceParams{TraceID: pcommon.TraceID{}}))
			require.NoError(t, err)
			assert.Empty(t, zero)
			services, err := store.GetServices(t.Context())
			require.NoError(t, err)
			if tc.last == 0 {
				assert.Empty(t, services)
			} else {
				assert.Equal(t, []string{"valid-service"}, services)
			}
			operations, err := store.GetOperations(t.Context(), tracestore.OperationQueryParams{ServiceName: "discarded-service"})
			require.NoError(t, err)
			assert.Empty(t, operations)
		})
	}
}

func TestWriteTraces_EmptyTraceIDMixedBatch(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		t.Run(fmt.Sprintf("readOnly=%v", readOnly), func(t *testing.T) {
			store, err := NewStore(Configuration{MaxTraces: 5})
			require.NoError(t, err)
			td := loadInputTraces(t, 1)
			for _, resource := range td.ResourceSpans().All() {
				for _, scope := range resource.ScopeSpans().All() {
					span := scope.Spans().AppendEmpty()
					span.SetSpanID(pcommon.SpanID{99})
					span.SetName("discarded-operation")
				}
			}
			if readOnly {
				td.MarkReadOnly()
			}
			marshaler := ptrace.JSONMarshaler{}
			before, err := marshaler.MarshalTraces(td)
			require.NoError(t, err)
			require.NoError(t, store.WriteTraces(t.Context(), td))
			after, err := marshaler.MarshalTraces(td)
			require.NoError(t, err)
			assert.Equal(t, before, after, "writing must not modify the input batch")
			assert.NotContains(t, store.getTenant("").ids, pcommon.TraceID{})
			for _, fixture := range []int{1, 2} {
				expected := loadOutputTraces(t, fixture)
				actual, err := jiter.FlattenWithErrors(store.GetTraces(t.Context(), tracestore.GetTraceParams{TraceID: jptrace.GetTraceID(expected)}))
				require.NoError(t, err)
				require.Len(t, actual, 1)
				assert.Equal(t, expected, actual[0])
			}
		})
	}
}
