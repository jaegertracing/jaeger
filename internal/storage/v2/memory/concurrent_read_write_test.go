// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// Exercise only the public storage API, with separately owned input per write.
func TestReproConcurrentTraceReadWrite(t *testing.T) {
	for _, method := range []string{"GetTraces", "FindTraces"} {
		t.Run(method, func(t *testing.T) {
			ctx := context.Background()
			store, err := NewStore(Configuration{MaxTraces: 10})
			require.NoError(t, err)
			id := pcommon.TraceID{1}
			newBatch := func(sequence int) ptrace.Traces {
				td := ptrace.NewTraces()
				rs := td.ResourceSpans().AppendEmpty()
				rs.Resource().Attributes().PutStr("service.name", "repro")
				span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
				span.SetTraceID(id)
				span.SetSpanID(pcommon.SpanID{byte(sequence), byte(sequence >> 8)})
				span.SetName("operation")
				span.SetStartTimestamp(1000000000)
				span.SetEndTimestamp(1000000001)
				return td
			}
			require.NoError(t, store.WriteTraces(ctx, newBatch(1)))
			start := make(chan struct{})
			var wg sync.WaitGroup
			wg.Go(func() {
				<-start
				for i := range 300 {
					if err := store.WriteTraces(ctx, newBatch(i+2)); err != nil {
						t.Error(err)
						return
					}
				}
			})
			wg.Go(func() {
				<-start
				for range 300 {
					read := store.GetTraces(ctx, tracestore.GetTraceParams{TraceID: id})
					if method == "FindTraces" {
						read = store.FindTraces(ctx, tracestore.TraceQueryParams{ServiceName: "repro", SearchDepth: 10, Attributes: pcommon.NewMap()})
					}
					for traces, err := range read {
						if err != nil {
							t.Error(err)
							return
						}
						if len(traces) != 1 || traces[0].SpanCount() == 0 {
							t.Error("expected nonempty trace")
						}
					}
				}
			})
			close(start)
			wg.Wait()
		})
	}
}
