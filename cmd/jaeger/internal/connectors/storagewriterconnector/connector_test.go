// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagewriterconnector

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/connector/connectortest"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// fakeWriter records the traces written and returns a configured error, standing in
// for the real synchronous ES writer that returns *esclient.BulkWriteError.
type fakeWriter struct {
	err     error
	written ptrace.Traces
	calls   int
}

func (f *fakeWriter) WriteTraces(_ context.Context, td ptrace.Traces) error {
	f.calls++
	f.written = td
	return f.err
}

func traceID(b byte) pcommon.TraceID { return pcommon.TraceID([16]byte{b}) }
func spanID(b byte) pcommon.SpanID   { return pcommon.SpanID([8]byte{b}) }

// makeSpan appends a span with the given ids/name to ss and returns its deterministic
// doc-id prefix (traceID_spanID) plus a full poison _id (…_hash) as the writer would
// emit it, so a test can build a RejectedItem that maps back to exactly this span.
func makeSpan(ss ptrace.ScopeSpans, tid pcommon.TraceID, sid pcommon.SpanID, name string) (span ptrace.Span, docID string) {
	span = ss.Spans().AppendEmpty()
	span.SetTraceID(tid)
	span.SetSpanID(sid)
	span.SetName(name)
	return span, tid.String() + "_" + sid.String() + "_deadbeef"
}

// makeTraces builds a batch with one resource/scope holding three spans (A, B, C) and
// returns the traces plus the three spans' poison _ids in order.
func makeTraces() (td ptrace.Traces, ids [3]string) {
	td = ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "svc")
	ss := rs.ScopeSpans().AppendEmpty()
	_, ids[0] = makeSpan(ss, traceID(1), spanID(1), "A")
	_, ids[1] = makeSpan(ss, traceID(2), spanID(2), "B")
	_, ids[2] = makeSpan(ss, traceID(3), spanID(3), "C")
	return td, ids
}

func newTestConnector(next consumer.Traces, w tracestore.Writer) *connectorImpl {
	set := connectortest.NewNopSettings(connectortest.NopType)
	c := newConnector(&Config{TraceStorage: "somestore"}, set.TelemetrySettings, next)
	c.traceWriter = w
	return c
}

func rejected(id string) esclient.RejectedItem {
	return esclient.RejectedItem{Index: "jaeger-span", ID: id, Status: 400, Reason: "mapper_parsing_exception"}
}

func TestConsumeTraces_Success(t *testing.T) {
	sink := new(consumertest.TracesSink)
	w := &fakeWriter{err: nil}
	c := newTestConnector(sink, w)
	td, _ := makeTraces()

	require.NoError(t, c.ConsumeTraces(context.Background(), td))
	assert.Equal(t, 1, w.calls)
	assert.Empty(t, sink.AllTraces(), "a fully successful write dead-letters nothing")
}

func TestConsumeTraces_NonBulkErrorRetries(t *testing.T) {
	sink := new(consumertest.TracesSink)
	sentinel := errors.New("connection refused")
	c := newTestConnector(sink, &fakeWriter{err: sentinel})
	td, _ := makeTraces()

	err := c.ConsumeTraces(context.Background(), td)
	require.ErrorIs(t, err, sentinel, "a transport error is returned so the offset is held")
	assert.Empty(t, sink.AllTraces(), "a non-poison error dead-letters nothing")
}

func TestConsumeTraces_TerminalOnlyDeadLetteredAndAdvances(t *testing.T) {
	sink := new(consumertest.TracesSink)
	td, ids := makeTraces()
	// A and C are poison; B succeeded. No transient failures.
	bulkErr := &esclient.BulkWriteError{Terminal: []esclient.RejectedItem{rejected(ids[0]), rejected(ids[2])}}
	c := newTestConnector(sink, &fakeWriter{err: bulkErr})

	err := c.ConsumeTraces(context.Background(), td)
	require.NoError(t, err, "all failures were terminal and dead-lettered, so the offset advances")

	require.Len(t, sink.AllTraces(), 1)
	got := sink.AllTraces()[0]
	assert.Equal(t, 2, got.SpanCount(), "only the two poison spans are emitted")
	names := spanNames(got)
	assert.ElementsMatch(t, []string{"A", "C"}, names, "exactly the poison spans, not B")
}

func TestConsumeTraces_TransientPresentRetries(t *testing.T) {
	sink := new(consumertest.TracesSink)
	td, ids := makeTraces()
	bulkErr := &esclient.BulkWriteError{
		Terminal:  []esclient.RejectedItem{rejected(ids[0])},
		Transient: true,
	}
	c := newTestConnector(sink, &fakeWriter{err: bulkErr})

	err := c.ConsumeTraces(context.Background(), td)
	require.Error(t, err, "a transient failure holds the offset for retry")

	// The poison span is still dead-lettered even though the batch is retried; the
	// deterministic _id keeps the re-send idempotent.
	require.Len(t, sink.AllTraces(), 1)
	assert.Equal(t, []string{"A"}, spanNames(sink.AllTraces()[0]))
}

func TestConsumeTraces_DeadLetterSinkFailureHoldsOffset(t *testing.T) {
	sinkErr := errors.New("kafka DLQ unavailable")
	td, ids := makeTraces()
	bulkErr := &esclient.BulkWriteError{Terminal: []esclient.RejectedItem{rejected(ids[0])}}
	c := newTestConnector(consumertest.NewErr(sinkErr), &fakeWriter{err: bulkErr})

	err := c.ConsumeTraces(context.Background(), td)
	require.ErrorIs(t, err, sinkErr, "if the dead-letter sink rejects, hold the offset")
}

func TestConsumeTraces_TransientBulkErrorNoTerminals(t *testing.T) {
	// drop mode returns a BulkWriteError with Transient=true and no Terminal items:
	// nothing to dead-letter, but the batch must still be retried.
	sink := new(consumertest.TracesSink)
	bulkErr := &esclient.BulkWriteError{Transient: true}
	c := newTestConnector(sink, &fakeWriter{err: bulkErr})
	td, _ := makeTraces()

	err := c.ConsumeTraces(context.Background(), td)
	require.Error(t, err)
	assert.Empty(t, sink.AllTraces(), "no terminal items means nothing is dead-lettered")
}

func TestFilterPoisonSpans_PreservesResourceAndScope(t *testing.T) {
	td := ptrace.NewTraces()
	rs1 := td.ResourceSpans().AppendEmpty()
	rs1.Resource().Attributes().PutStr("service.name", "svc1")
	ss1 := rs1.ScopeSpans().AppendEmpty()
	ss1.Scope().SetName("scope1")
	_, idA := makeSpan(ss1, traceID(1), spanID(1), "A")
	makeSpan(ss1, traceID(2), spanID(2), "B") // not poison

	rs2 := td.ResourceSpans().AppendEmpty()
	rs2.Resource().Attributes().PutStr("service.name", "svc2")
	ss2 := rs2.ScopeSpans().AppendEmpty()
	ss2.Scope().SetName("scope2")
	_, idC := makeSpan(ss2, traceID(3), spanID(3), "C")

	out := filterPoisonSpans(td, []esclient.RejectedItem{rejected(idA), rejected(idC)})

	require.Equal(t, 2, out.ResourceSpans().Len(), "both resources are preserved for their poison spans")
	got1 := out.ResourceSpans().At(0)
	svc, _ := got1.Resource().Attributes().Get("service.name")
	assert.Equal(t, "svc1", svc.AsString())
	assert.Equal(t, "scope1", got1.ScopeSpans().At(0).Scope().Name())
	assert.Equal(t, "A", got1.ScopeSpans().At(0).Spans().At(0).Name())
	got2 := out.ResourceSpans().At(1)
	assert.Equal(t, "C", got2.ScopeSpans().At(0).Spans().At(0).Name())
}

func TestFilterPoisonSpans_EmptyWhenNoMappableIDs(t *testing.T) {
	td, _ := makeTraces()
	// A service:operation doc _id has no traceID_spanID_ shape, so it maps to nothing.
	out := filterPoisonSpans(td, []esclient.RejectedItem{{ID: "serviceoperationdoc"}})
	assert.Equal(t, 0, out.SpanCount())
}

func TestSpanKeyFromID(t *testing.T) {
	key, ok := spanKeyFromID("aabb_ccdd_deadbeef")
	require.True(t, ok)
	assert.Equal(t, "aabb_ccdd", key)

	_, ok = spanKeyFromID("noseparators")
	assert.False(t, ok)

	_, ok = spanKeyFromID("only_one")
	assert.False(t, ok, "a service doc id with a single underscore is not a span id")
}

func TestConnector_StartResolvesWriter(t *testing.T) {
	c := newTestConnector(new(consumertest.TracesSink), nil)
	want := &fakeWriter{}
	c.resolveWriter = func(component.Host) (tracestore.Writer, error) { return want, nil }

	require.NoError(t, c.Start(context.Background(), componenttest.NewNopHost()))
	assert.Same(t, want, c.traceWriter)
	require.NoError(t, c.Shutdown(context.Background()))
	assert.False(t, c.Capabilities().MutatesData)
}

func TestConnector_StartError(t *testing.T) {
	c := newTestConnector(new(consumertest.TracesSink), nil)
	boom := errors.New("no storage factory")
	c.resolveWriter = func(component.Host) (tracestore.Writer, error) { return nil, boom }
	require.ErrorIs(t, c.Start(context.Background(), componenttest.NewNopHost()), boom)
}

func spanNames(td ptrace.Traces) []string {
	var names []string
	for _, rs := range td.ResourceSpans().All() {
		for _, ss := range rs.ScopeSpans().All() {
			for _, s := range ss.Spans().All() {
				names = append(names, s.Name())
			}
		}
	}
	return names
}
