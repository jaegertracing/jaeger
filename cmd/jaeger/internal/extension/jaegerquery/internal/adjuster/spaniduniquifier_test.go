// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
)

var (
	clientSpanID  = pcommon.SpanID([]byte{0, 0, 0, 0, 0, 0, 0, 1})
	anotherSpanID = pcommon.SpanID([]byte{1, 0, 0, 0, 0, 0, 0, 0})
)

func makeTraces() ptrace.Traces {
	traceID := pcommon.TraceID([]byte{0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 3})

	traces := ptrace.NewTraces()
	spans := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans()

	clientSpan := spans.AppendEmpty()
	clientSpan.SetTraceID(traceID)
	clientSpan.SetSpanID(clientSpanID)
	clientSpan.SetKind(ptrace.SpanKindClient)

	serverSpan := spans.AppendEmpty()
	serverSpan.SetTraceID(traceID)
	serverSpan.SetSpanID(clientSpanID) // shared span ID
	serverSpan.SetKind(ptrace.SpanKindServer)

	anotherSpan := spans.AppendEmpty()
	anotherSpan.SetTraceID(traceID)
	anotherSpan.SetSpanID(anotherSpanID)
	anotherSpan.SetParentSpanID(clientSpanID)

	return traces
}

func TestSpanIDUniquifierTriggered(t *testing.T) {
	traces := makeTraces()
	deduper := DeduplicateClientServerSpanIDs()
	deduper.Adjust(traces)

	spans := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans()

	clientSpan := spans.At(0)
	assert.Equal(t, clientSpanID, clientSpan.SpanID(), "client span ID should not change")

	serverSpan := spans.At(1)
	assert.EqualValues(t, []byte{0, 0, 0, 0, 0, 0, 0, 2}, serverSpan.SpanID(), "server span ID should be reassigned")
	assert.Equal(t, clientSpanID, serverSpan.ParentSpanID(), "next server span should be this server span's parent")

	thirdSpan := spans.At(2)
	assert.Equal(t, anotherSpanID, thirdSpan.SpanID(), "3rd span ID should not change")
	assert.Equal(t, serverSpan.SpanID(), thirdSpan.ParentSpanID(), "parent of 3rd span should change to new spanID")
}

func TestSpanIDUniquifierNotTriggered(t *testing.T) {
	traces := makeTraces()
	spans := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans()

	// only copy server span and random span
	newSpans := ptrace.NewSpanSlice()
	spans.At(1).CopyTo(newSpans.AppendEmpty())
	spans.At(2).CopyTo(newSpans.AppendEmpty())
	newSpans.CopyTo(spans)

	deduper := DeduplicateClientServerSpanIDs()
	deduper.Adjust(traces)

	gotSpans := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans()

	serverSpanID := clientSpanID // for better readability
	serverSpan := gotSpans.At(0)
	assert.Equal(t, serverSpanID, serverSpan.SpanID(), "server span ID should be unchanged")

	thirdSpan := gotSpans.At(1)
	assert.Equal(t, anotherSpanID, thirdSpan.SpanID(), "3rd span ID should not change")
}

func TestSpanIDUniquifierError(t *testing.T) {
	traces := makeTraces()

	maxID := pcommon.SpanID([8]byte{255, 255, 255, 255, 255, 255, 255, 255})

	deduper := &spanIDDeduper{
		hasClientSpanByID: make(map[pcommon.SpanID]bool),
		// instead of 0 start at the last possible value to cause an error
		maxUsedID: maxID,
	}
	deduper.adjust(traces)

	span := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(1)
	warnings := jptrace.GetWarnings(span)
	require.Equal(t, []string{"cannot assign unique span ID, too many spans in the trace"}, warnings)
}

// makeSharedSpanIDTraces returns numServers server spans that all use clientSpanID,
// followed by a client span with the same ID when withClient is set.
func makeSharedSpanIDTraces(numServers int, withClient bool) ptrace.Traces {
	traces := ptrace.NewTraces()
	spans := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans()
	for range numServers {
		serverSpan := spans.AppendEmpty()
		serverSpan.SetSpanID(clientSpanID)
		serverSpan.SetKind(ptrace.SpanKindServer)
	}
	if withClient {
		clientSpan := spans.AppendEmpty()
		clientSpan.SetSpanID(clientSpanID)
		clientSpan.SetKind(ptrace.SpanKindClient)
	}
	return traces
}

func countDistinctSpanIDs(spans ptrace.SpanSlice) int {
	ids := make(map[pcommon.SpanID]struct{}, spans.Len())
	for i := 0; i < spans.Len(); i++ {
		ids[spans.At(i).SpanID()] = struct{}{}
	}
	return len(ids)
}

// adjustWithin fails the test if deduplicating the span IDs of traces takes longer than budget.
func adjustWithin(t *testing.T, traces ptrace.Traces, budget time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		DeduplicateClientServerSpanIDs().Adjust(traces)
	}()
	select {
	case <-done:
	case <-time.After(budget):
		t.Fatalf("deduplicating span IDs took longer than %v", budget)
	}
}

func TestSpanIDUniquifierManySpansShareID(t *testing.T) {
	const numServers = 50_000

	// The same number of spans with distinct IDs sets the time budget, so that it
	// scales with the machine and with -race. The floor absorbs scheduling noise
	// when that baseline takes only a few milliseconds.
	distinctIDs := makeSharedSpanIDTraces(numServers, false)
	distinctSpans := distinctIDs.ResourceSpans().At(0).ScopeSpans().At(0).Spans()
	spanID := pcommon.NewSpanIDEmpty()
	for i := 0; i < distinctSpans.Len(); i++ {
		spanID = incrementSpanID(spanID)
		distinctSpans.At(i).SetSpanID(spanID)
	}
	start := time.Now()
	DeduplicateClientServerSpanIDs().Adjust(distinctIDs)
	budget := max(20*time.Since(start), 250*time.Millisecond)

	t.Run("with client span", func(t *testing.T) {
		traces := makeSharedSpanIDTraces(numServers, true)
		adjustWithin(t, traces, budget)

		spans := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans()
		assert.Equal(t, clientSpanID, spans.At(numServers).SpanID(), "client span ID should not change")
		assert.Equal(t, numServers+1, countDistinctSpanIDs(spans), "every server span should get its own span ID")
	})

	t.Run("without client span", func(t *testing.T) {
		traces := makeSharedSpanIDTraces(numServers, false)
		adjustWithin(t, traces, budget)

		spans := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans()
		assert.Equal(t, clientSpanID, spans.At(0).SpanID(), "server span ID should not change")
		assert.Equal(t, 1, countDistinctSpanIDs(spans), "server span IDs should not change")
	})
}

func BenchmarkDeduplicateClientServerSpanIDs(b *testing.B) {
	for _, numServers := range []int{1_000, 10_000, 100_000} {
		b.Run("servers="+strconv.Itoa(numServers), func(b *testing.B) {
			traces := makeSharedSpanIDTraces(numServers, true)
			spans := traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans()
			deduper := DeduplicateClientServerSpanIDs()
			for b.Loop() {
				b.StopTimer()
				// restore the shared IDs that the previous iteration replaced
				for i := range numServers {
					spans.At(i).SetSpanID(clientSpanID)
					spans.At(i).SetParentSpanID(pcommon.NewSpanIDEmpty())
				}
				b.StartTimer()
				deduper.Adjust(traces)
			}
		})
	}
}
