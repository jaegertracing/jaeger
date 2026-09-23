// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storageexporter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/storagetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/connector/connectortest"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
)

// fakeWriter records the traces written and returns a configured error, standing in
// for a synchronous writer that returns *tracestore.RejectedSpansError.
// fakeWriter returns err on every call, or the entries of errs in order (the last
// one repeating) when errs is set, and remembers the span count of the last batch.
type fakeWriter struct {
	err       error
	errs      []error
	calls     int
	lastSpans int
}

func (f *fakeWriter) WriteTraces(_ context.Context, td ptrace.Traces) error {
	f.calls++
	f.lastSpans = td.SpanCount()
	if len(f.errs) == 0 {
		return f.err
	}
	if f.calls <= len(f.errs) {
		return f.errs[f.calls-1]
	}
	return f.errs[len(f.errs)-1]
}

// connectorStorageExt is a minimal jaeger_storage extension serving one named
// trace-store factory, so start resolves the writer through a real host without a
// backend.
type connectorStorageExt struct {
	name    string
	factory tracestore.Factory
}

var _ jaegerstorage.Extension = (*connectorStorageExt)(nil)

func (*connectorStorageExt) Start(context.Context, component.Host) error { return nil }
func (*connectorStorageExt) Shutdown(context.Context) error              { return nil }

func (m *connectorStorageExt) TraceStorageFactory(name string) (tracestore.Factory, error) {
	if m.name == name {
		return m.factory, nil
	}
	return nil, errors.New("storage not found")
}

func (*connectorStorageExt) MetricStorageFactory(string) (storage.MetricStoreFactory, error) {
	return nil, errors.New("metric storage not found")
}

// writerFactory returns a factory whose CreateTraceWriter yields w.
func writerFactory(w tracestore.Writer) *tracestoremocks.Factory {
	f := new(tracestoremocks.Factory)
	f.On("CreateTraceWriter").Return(w, nil)
	return f
}

func hostWith(name string, f tracestore.Factory) component.Host {
	return storagetest.NewStorageHost().WithExtension(jaegerstorage.ID, &connectorStorageExt{name: name, factory: f})
}

type testConnector struct {
	*connectorImpl
	logs   *observer.ObservedLogs
	reader *sdkmetric.ManualReader
}

// deadLetterSpans reads the connector's dead_letter_spans counter.
func (c testConnector) deadLetterSpans(t *testing.T) int64 {
	var rm metricdata.ResourceMetrics
	require.NoError(t, c.reader.Collect(context.Background(), &rm))
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "jaeger_storage_exporter_dead_letter_spans" {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok)
			require.Len(t, sum.DataPoints, 1)
			return sum.DataPoints[0].Value
		}
	}
	return 0
}

// newTestConnector builds a connector on cfg, starts it against a host serving a
// poison-reporting factory that hands out w, and stops it when the test ends.
func newTestConnector(t *testing.T, cfg *Config, next consumer.Traces, w tracestore.Writer) testConnector {
	core, logs := observer.New(zapcore.WarnLevel)
	reader := sdkmetric.NewManualReader()
	set := connectortest.NewNopSettings(componentType)
	set.Logger = zap.New(core)
	set.MeterProvider = sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	conn, err := createTracesToTraces(context.Background(), set, cfg, next)
	require.NoError(t, err)
	c := conn.(*connectorImpl)
	require.NoError(t, c.Start(context.Background(), hostWith(cfg.TraceStorage, writerFactory(w))))
	t.Cleanup(func() { require.NoError(t, c.Shutdown(context.Background())) })
	return testConnector{connectorImpl: c, logs: logs, reader: reader}
}

func directConfig() *Config {
	cfg := createDefaultConfig().(*Config)
	cfg.TraceStorage = "somestore"
	return cfg
}

func traceID(b byte) pcommon.TraceID { return pcommon.TraceID([16]byte{b}) }
func spanID(b byte) pcommon.SpanID   { return pcommon.SpanID([8]byte{b}) }

// makeSpan appends a span with the given ids/name to ss and returns it along with
// the rejection a writer would report for it.
func makeSpan(ss ptrace.ScopeSpans, tid pcommon.TraceID, sid pcommon.SpanID, name string) (span ptrace.Span, rejection tracestore.RejectedSpan) {
	span = ss.Spans().AppendEmpty()
	span.SetTraceID(tid)
	span.SetSpanID(sid)
	span.SetName(name)
	return span, tracestore.RejectedSpan{TraceID: tid, SpanID: sid, Reason: "mapper_parsing_exception"}
}

// makeTraces builds a batch with one resource/scope holding three spans (A, B, C) and
// returns the traces plus the three spans' rejections in order.
func makeTraces() (td ptrace.Traces, ids [3]tracestore.RejectedSpan) {
	td = ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "svc")
	ss := rs.ScopeSpans().AppendEmpty()
	_, ids[0] = makeSpan(ss, traceID(1), spanID(1), "A")
	_, ids[1] = makeSpan(ss, traceID(2), spanID(2), "B")
	_, ids[2] = makeSpan(ss, traceID(3), spanID(3), "C")
	return td, ids
}

// rejectedErr builds the error a writer returns for the given rejected spans.
func rejectedErr(transient bool, spans ...tracestore.RejectedSpan) *tracestore.RejectedSpansError {
	return &tracestore.RejectedSpansError{Spans: spans, Transient: transient, Err: errors.New("2 of 3 bulk items rejected")}
}

func spansOf(td ptrace.Traces) []ptrace.Span {
	var spans []ptrace.Span
	for _, rs := range td.ResourceSpans().All() {
		for _, ss := range rs.ScopeSpans().All() {
			for _, s := range ss.Spans().All() {
				spans = append(spans, s)
			}
		}
	}
	return spans
}

func spanNames(td ptrace.Traces) []string {
	var names []string
	for _, s := range spansOf(td) {
		names = append(names, s.Name())
	}
	return names
}

func TestConsumeTraces_Success(t *testing.T) {
	sink := new(consumertest.TracesSink)
	w := &fakeWriter{}
	c := newTestConnector(t, directConfig(), sink, w)
	td, _ := makeTraces()

	require.NoError(t, c.ConsumeTraces(context.Background(), td))
	assert.Equal(t, 1, w.calls)
	assert.Equal(t, td.SpanCount(), w.lastSpans, "the writer receives the whole batch")
	assert.Empty(t, sink.AllTraces(), "a fully successful write sends nothing to the dead-letter pipeline")
}

func TestConsumeTraces_NonBulkErrorRetries(t *testing.T) {
	sink := new(consumertest.TracesSink)
	sentinel := errors.New("connection refused")
	c := newTestConnector(t, directConfig(), sink, &fakeWriter{err: sentinel})
	td, _ := makeTraces()

	err := c.ConsumeTraces(context.Background(), td)
	require.ErrorIs(t, err, sentinel, "a transport error is returned so the offset is held")
	assert.Empty(t, sink.AllTraces(), "a non-poison error sends nothing to the dead-letter pipeline")
}

func TestConsumeTraces_TerminalOnlyGoesToDeadLetterAndAdvances(t *testing.T) {
	sink := new(consumertest.TracesSink)
	td, ids := makeTraces()
	// A and C are poison; B succeeded. No transient failures.
	c := newTestConnector(t, directConfig(), sink, &fakeWriter{err: rejectedErr(false, ids[0], ids[2])})

	err := c.ConsumeTraces(context.Background(), td)
	require.NoError(t, err, "all failures were terminal and re-routed, so the offset advances")

	require.Len(t, sink.AllTraces(), 1)
	got := sink.AllTraces()[0]
	assert.Equal(t, 2, got.SpanCount(), "only the two poison spans are emitted")
	assert.ElementsMatch(t, []string{"A", "C"}, spanNames(got), "exactly the poison spans, not B")
	for _, span := range spansOf(got) {
		reason, ok := span.Attributes().Get(rejectionReasonAttribute)
		require.True(t, ok, "every re-emitted span carries its rejection reason")
		assert.Equal(t, "mapper_parsing_exception", reason.Str())
	}
	_, tagged := spansOf(td)[0].Attributes().Get(rejectionReasonAttribute)
	assert.False(t, tagged, "the reason is set on the copy, not on the input span")
	require.Equal(t, 2, c.logs.Len(), "one log per poison span")
	for _, entry := range c.logs.All() {
		assert.Equal(t, "storage rejected span terminally; sent it to the dead-letter pipeline", entry.Message)
		assert.Equal(t, "mapper_parsing_exception", entry.ContextMap()["reason"])
	}
	assert.Equal(t, int64(2), c.deadLetterSpans(t), "the counter reports the two re-routed spans")
}

// TestConsumeTraces_RetryRoutesPoisonOnce is the retry claim end to end: the first
// attempt sees a transient failure alongside the poison span and is retried by
// retry_on_failure; the retry sees only the terminal rejection, and only then does
// the poison span go to the dead-letter pipeline, exactly once.
func TestConsumeTraces_RetryRoutesPoisonOnce(t *testing.T) {
	cfg := directConfig()
	cfg.RetryConfig.Enabled = true
	cfg.RetryConfig.InitialInterval = time.Millisecond
	cfg.RetryConfig.MaxInterval = time.Millisecond
	sink := new(consumertest.TracesSink)
	td, ids := makeTraces()
	w := &fakeWriter{errs: []error{rejectedErr(true, ids[0]), rejectedErr(false, ids[0])}}
	c := newTestConnector(t, cfg, sink, w)

	require.NoError(t, c.ConsumeTraces(context.Background(), td))
	assert.Equal(t, 2, w.calls, "the transient failure was retried once")
	require.Len(t, sink.AllTraces(), 1, "the poison span reached the dead-letter pipeline once")
	assert.Equal(t, []string{"A"}, spanNames(sink.AllTraces()[0]))
	assert.Equal(t, int64(1), c.deadLetterSpans(t))
}

func TestConsumeTraces_TransientPresentRetriesWithoutReRouting(t *testing.T) {
	sink := new(consumertest.TracesSink)
	td, ids := makeTraces()
	rejected := rejectedErr(true, ids[0])
	c := newTestConnector(t, directConfig(), sink, &fakeWriter{err: rejected})

	err := c.ConsumeTraces(context.Background(), td)
	require.ErrorIs(t, err, rejected, "a transient failure holds the offset for retry")
	// The poison is not re-routed yet: the batch will be retried, and the poison goes
	// to the dead-letter pipeline once a retry sees only terminal failures, so a
	// retry storm never sends the same span there twice.
	assert.Empty(t, sink.AllTraces())
}

func TestConsumeTraces_DeadLetterSinkFailureHoldsOffset(t *testing.T) {
	td, ids := makeTraces()
	// The sink's exporter classifies a 4xx from its endpoint as permanent; the
	// connector must still return a retryable error, or its exporterhelper would
	// skip retries and the receiver would pause the partition.
	sinkErr := consumererror.NewPermanent(errors.New("dead-letter endpoint returned 401"))
	c := newTestConnector(t, directConfig(), consumertest.NewErr(sinkErr), &fakeWriter{err: rejectedErr(false, ids[0])})

	err := c.ConsumeTraces(context.Background(), td)
	require.Error(t, err, "if the dead-letter sink rejects, hold the offset")
	require.ErrorContains(t, err, "dead-letter pipeline rejected 1 poison spans: Permanent error: dead-letter endpoint returned 401")
	assert.False(t, consumererror.IsPermanent(err), "a sink failure is retryable for the storage write")
	for _, entry := range c.logs.All() {
		assert.NotEqual(t, "storage rejected span terminally; sent it to the dead-letter pipeline", entry.Message,
			"nothing is logged as sent until the sink accepts")
	}
	assert.Zero(t, c.deadLetterSpans(t))
}

func TestConsumeTraces_DuplicateRejectionLoggedOnce(t *testing.T) {
	sink := new(consumertest.TracesSink)
	td, ids := makeTraces()
	twice := ids[0]
	twice.Reason = "another reason for the same span"
	c := newTestConnector(t, directConfig(), sink, &fakeWriter{err: rejectedErr(false, ids[0], twice)})

	require.NoError(t, c.ConsumeTraces(context.Background(), td))
	require.Len(t, sink.AllTraces(), 1)
	assert.Equal(t, []string{"A"}, spanNames(sink.AllTraces()[0]), "the span is re-emitted once")
	reason, _ := spansOf(sink.AllTraces()[0])[0].Attributes().Get(rejectionReasonAttribute)
	assert.Equal(t, ids[0].Reason, reason.Str(), "the first report's reason is attached")
	require.Equal(t, 1, c.logs.Len(), "and logged once")
	assert.Equal(t, ids[0].Reason, c.logs.All()[0].ContextMap()["reason"], "with the same reason")
	assert.Equal(t, int64(1), c.deadLetterSpans(t))
}

func TestConsumeTraces_TransientRejectionWithoutSpans(t *testing.T) {
	// A storage may report transient failures through a RejectedSpansError that
	// names no spans: nothing to re-route, but the batch must still be retried.
	sink := new(consumertest.TracesSink)
	rejected := rejectedErr(true)
	c := newTestConnector(t, directConfig(), sink, &fakeWriter{err: rejected})
	td, _ := makeTraces()

	require.ErrorIs(t, c.ConsumeTraces(context.Background(), td), rejected)
	assert.Empty(t, sink.AllTraces(), "no rejected spans means nothing goes to the dead-letter pipeline")
}

func TestConsumeTraces_NoSpansRejectedAdvances(t *testing.T) {
	// The storage handled every rejected document itself (a lookup document it
	// logged): nothing to re-route, and the batch is complete.
	sink := new(consumertest.TracesSink)
	c := newTestConnector(t, directConfig(), sink, &fakeWriter{err: rejectedErr(false)})
	td, _ := makeTraces()

	require.NoError(t, c.ConsumeTraces(context.Background(), td))
	assert.Empty(t, sink.AllTraces())
	assert.Zero(t, c.logs.Len())
}

func TestConsumeTraces_UnidentifiedRejectionFailsTheBatch(t *testing.T) {
	sink := new(consumertest.TracesSink)
	td, _ := makeTraces()
	// The storage rejected a document it could not attribute to a span; it could be
	// a span, so acknowledging would lose it silently.
	rejected := &tracestore.RejectedSpansError{Unidentified: 1, Err: errors.New("1 of 3 bulk items rejected")}
	c := newTestConnector(t, directConfig(), sink, &fakeWriter{err: rejected})

	err := c.ConsumeTraces(context.Background(), td)
	require.ErrorIs(t, err, rejected)
	require.ErrorContains(t, err, "1 rejected documents could not be attributed to a span")
	assert.False(t, consumererror.IsPermanent(err), "the batch is retried, not dropped by the pipeline")
	assert.Empty(t, sink.AllTraces())
}

func TestConsumeTraces_RejectedSpanNotInBatchFailsTheBatch(t *testing.T) {
	sink := new(consumertest.TracesSink)
	td, ids := makeTraces()
	stranger := tracestore.RejectedSpan{TraceID: traceID(9), SpanID: spanID(9), Reason: "mapper_parsing_exception"}
	c := newTestConnector(t, directConfig(), sink, &fakeWriter{err: rejectedErr(false, ids[0], stranger)})

	err := c.ConsumeTraces(context.Background(), td)
	require.ErrorContains(t, err, "1 rejected spans are not in the batch")
	assert.False(t, consumererror.IsPermanent(err), "the batch is retried, not dropped")
	assert.Empty(t, sink.AllTraces(), "nothing is re-routed until every rejected span is accounted for")
	assert.Zero(t, c.deadLetterSpans(t))
}

// TestConsumeTraces_BlockingQueueMergesCallers is the README's central claim: two
// concurrent ConsumeTraces calls are merged into one storage write, each caller
// gets that write's verdict, and the poison from both is re-routed in one send.
func TestConsumeTraces_BlockingQueueMergesCallers(t *testing.T) {
	td1, ids1 := makeTraces()
	td2 := ptrace.NewTraces()
	ss := td2.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
	_, d := makeSpan(ss, traceID(4), spanID(4), "D")
	makeSpan(ss, traceID(5), spanID(5), "E")

	queue := exporterhelper.NewDefaultQueueConfig()
	queue.WaitForResult = true
	queue.NumConsumers = 1
	queue.Batch = configoptional.Some(exporterhelper.BatchConfig{
		Sizer:        exporterhelper.RequestSizerTypeItems,
		FlushTimeout: 5 * time.Second,
		MinSize:      int64(td1.SpanCount() + td2.SpanCount()),
	})
	cfg := directConfig()
	cfg.QueueConfig = configoptional.Some(queue)

	sink := new(consumertest.TracesSink)
	w := &fakeWriter{err: rejectedErr(false, ids1[0], d)}
	c := newTestConnector(t, cfg, sink, w)

	errs := make(chan error, 2)
	for _, td := range []ptrace.Traces{td1, td2} {
		go func() { errs <- c.ConsumeTraces(context.Background(), td) }()
	}
	for range 2 {
		require.NoError(t, <-errs, "each caller gets the merged write's verdict")
	}
	assert.Equal(t, 1, w.calls, "both callers were merged into one write")
	assert.Equal(t, 5, w.lastSpans, "the writer saw the merged batch")
	require.Len(t, sink.AllTraces(), 1, "poison from both callers goes out in one send")
	assert.ElementsMatch(t, []string{"A", "D"}, spanNames(sink.AllTraces()[0]))
	assert.Equal(t, int64(2), c.deadLetterSpans(t))
	assert.True(t, c.Capabilities().MutatesData, "the batcher moves spans out of the caller's input, and the connector reports the pipeline's capabilities")
}

// TestConsumeTraces_BlockingQueueReturnsWriteVerdict proves the connector wraps the
// exporter pipeline: with queue.wait_for_result and a batch configured, the
// caller's ConsumeTraces still returns the storage write's verdict (RFC 0007 §4.2).
func TestConsumeTraces_BlockingQueueReturnsWriteVerdict(t *testing.T) {
	queue := exporterhelper.NewDefaultQueueConfig()
	queue.WaitForResult = true
	queue.NumConsumers = 1
	queue.Batch = configoptional.Some(exporterhelper.BatchConfig{
		Sizer:        exporterhelper.RequestSizerTypeItems,
		FlushTimeout: 10 * time.Millisecond,
		MinSize:      1,
	})
	cfg := directConfig()
	cfg.QueueConfig = configoptional.Some(queue)

	td, ids := makeTraces()
	sink := new(consumertest.TracesSink)
	c := newTestConnector(t, cfg, sink, &fakeWriter{err: rejectedErr(false, ids[1])})

	require.NoError(t, c.ConsumeTraces(context.Background(), td), "the caller is unblocked with the batch's verdict")
	require.Len(t, sink.AllTraces(), 1)
	assert.Equal(t, []string{"B"}, spanNames(sink.AllTraces()[0]))

	sentinel := errors.New("backend down")
	c2 := newTestConnector(t, cfg, sink, &fakeWriter{err: sentinel})
	require.ErrorIs(t, c2.ConsumeTraces(context.Background(), td), sentinel, "the caller sees the write's error, not an enqueue ack")
}

func TestSelectSpans_PreservesResourceAndScope(t *testing.T) {
	td := ptrace.NewTraces()
	rs1 := td.ResourceSpans().AppendEmpty()
	rs1.Resource().Attributes().PutStr("service.name", "svc1")
	ss1 := rs1.ScopeSpans().AppendEmpty()
	ss1.Scope().SetName("scope1")
	_, a := makeSpan(ss1, traceID(1), spanID(1), "A")
	makeSpan(ss1, traceID(2), spanID(2), "B") // not rejected

	rs2 := td.ResourceSpans().AppendEmpty()
	rs2.Resource().Attributes().PutStr("service.name", "svc2")
	ss2 := rs2.ScopeSpans().AppendEmpty()
	ss2.Scope().SetName("scope2")
	_, c := makeSpan(ss2, traceID(3), spanID(3), "C")

	out, _, unmatched := selectSpans(td, []tracestore.RejectedSpan{a, c})
	require.Zero(t, unmatched)

	require.Equal(t, 2, out.ResourceSpans().Len(), "both resources are preserved for their rejected spans")
	got1 := out.ResourceSpans().At(0)
	svc, _ := got1.Resource().Attributes().Get("service.name")
	assert.Equal(t, "svc1", svc.AsString())
	assert.Equal(t, "scope1", got1.ScopeSpans().At(0).Scope().Name())
	assert.Equal(t, "A", got1.ScopeSpans().At(0).Spans().At(0).Name())
	got2 := out.ResourceSpans().At(1)
	assert.Equal(t, "C", got2.ScopeSpans().At(0).Spans().At(0).Name())
}

func TestSelectSpans_EmptyWhenNothingRejected(t *testing.T) {
	td, _ := makeTraces()
	out, _, unmatched := selectSpans(td, nil)
	assert.Equal(t, 0, out.SpanCount())
	assert.Zero(t, unmatched)
}

func TestSelectSpans_SharedSpanIDsAreBothSelected(t *testing.T) {
	// Two spans share trace and span id (a client and a server span); only one is
	// poison, but the ids cannot tell them apart, so both are re-routed.
	td := ptrace.NewTraces()
	ss := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
	_, client := makeSpan(ss, traceID(1), spanID(1), "client")
	makeSpan(ss, traceID(1), spanID(1), "server")
	makeSpan(ss, traceID(2), spanID(2), "other")

	out, unique, unmatched := selectSpans(td, []tracestore.RejectedSpan{client, client})
	assert.Len(t, unique, 1, "a span reported twice is one rejection")
	assert.ElementsMatch(t, []string{"client", "server"}, spanNames(out))
	assert.Zero(t, unmatched, "one rejected id, matched by both spans")
}

func TestStart_StorageFactoryNotFound(t *testing.T) {
	conn, err := createTracesToTraces(context.Background(), connectortest.NewNopSettings(componentType), directConfig(), consumertest.NewNop())
	require.NoError(t, err)
	err = conn.Start(context.Background(), hostWith("othername", nil))
	require.ErrorContains(t, err, "cannot find storage factory")
}

func TestStart_CreateTraceWriterError(t *testing.T) {
	factory := new(tracestoremocks.Factory)
	factory.On("CreateTraceWriter").Return(nil, errors.New("boom"))
	conn, err := createTracesToTraces(context.Background(), connectortest.NewNopSettings(componentType), directConfig(), consumertest.NewNop())
	require.NoError(t, err)
	err = conn.Start(context.Background(), hostWith("somestore", factory))
	require.ErrorContains(t, err, "cannot create trace writer")
}

func TestStart_ResolvesWriter(t *testing.T) {
	c := newTestConnector(t, directConfig(), consumertest.NewNop(), &fakeWriter{})
	assert.False(t, c.Capabilities().MutatesData)
}
