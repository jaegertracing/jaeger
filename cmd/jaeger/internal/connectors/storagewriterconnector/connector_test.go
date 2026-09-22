// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagewriterconnector

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
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
)

// fakeWriter records the traces written and returns a configured error, standing in
// for the real synchronous ES writer that returns *esclient.BulkWriteError.
type fakeWriter struct {
	err   error
	calls int
}

func (f *fakeWriter) WriteTraces(context.Context, ptrace.Traces) error {
	f.calls++
	return f.err
}

// mockStorageExt is a minimal jaeger_storage extension serving one named trace-store
// factory, so start resolves the writer through a real host without a backend.
type mockStorageExt struct {
	name    string
	factory tracestore.Factory
}

var _ jaegerstorage.Extension = (*mockStorageExt)(nil)

func (*mockStorageExt) Start(context.Context, component.Host) error { return nil }
func (*mockStorageExt) Shutdown(context.Context) error              { return nil }

func (m *mockStorageExt) TraceStorageFactory(name string) (tracestore.Factory, error) {
	if m.name == name {
		return m.factory, nil
	}
	return nil, errors.New("storage not found")
}

func (*mockStorageExt) MetricStorageFactory(string) (storage.MetricStoreFactory, error) {
	return nil, errors.New("metric storage not found")
}

// reportingFactory is a trace-store factory that also implements
// tracestore.PoisonPillReporting, as the ES factory does.
type reportingFactory struct {
	tracestore.Factory
	reports bool
}

func (f reportingFactory) ReportsPoisonPills() bool { return f.reports }

// writerFactory returns a factory whose CreateTraceWriter yields w.
func writerFactory(w tracestore.Writer) *tracestoremocks.Factory {
	f := new(tracestoremocks.Factory)
	f.On("CreateTraceWriter").Return(w, nil)
	return f
}

func hostWith(name string, f tracestore.Factory) component.Host {
	return storagetest.NewStorageHost().WithExtension(jaegerstorage.ID, &mockStorageExt{name: name, factory: f})
}

type testConnector struct {
	*connectorImpl
	logs   *observer.ObservedLogs
	reader *sdkmetric.ManualReader
}

// deadLettered reads the connector's dead_lettered_spans counter.
func (c testConnector) deadLettered(t *testing.T) int64 {
	var rm metricdata.ResourceMetrics
	require.NoError(t, c.reader.Collect(context.Background(), &rm))
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "jaeger_storage_writer_dead_lettered_spans" {
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
	require.NoError(t, c.Start(context.Background(), hostWith(cfg.TraceStorage, reportingFactory{Factory: writerFactory(w), reports: true})))
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

// makeSpan appends a span with the given ids/name to ss and returns the poison _id
// (traceID_spanID_hash) the writer would report for it.
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

func rejected(id string) esclient.RejectedItem {
	return esclient.RejectedItem{Index: "jaeger-span", ID: id, Status: 400, Reason: "mapper_parsing_exception"}
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

func TestConsumeTraces_Success(t *testing.T) {
	sink := new(consumertest.TracesSink)
	w := &fakeWriter{}
	c := newTestConnector(t, directConfig(), sink, w)
	td, _ := makeTraces()

	require.NoError(t, c.ConsumeTraces(context.Background(), td))
	assert.Equal(t, 1, w.calls)
	assert.Empty(t, sink.AllTraces(), "a fully successful write dead-letters nothing")
}

func TestConsumeTraces_NonBulkErrorRetries(t *testing.T) {
	sink := new(consumertest.TracesSink)
	sentinel := errors.New("connection refused")
	c := newTestConnector(t, directConfig(), sink, &fakeWriter{err: sentinel})
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
	c := newTestConnector(t, directConfig(), sink, &fakeWriter{err: bulkErr})

	err := c.ConsumeTraces(context.Background(), td)
	require.NoError(t, err, "all failures were terminal and dead-lettered, so the offset advances")

	require.Len(t, sink.AllTraces(), 1)
	got := sink.AllTraces()[0]
	assert.Equal(t, 2, got.SpanCount(), "only the two poison spans are emitted")
	assert.ElementsMatch(t, []string{"A", "C"}, spanNames(got), "exactly the poison spans, not B")
	require.Equal(t, 1, c.logs.Len())
	assert.Equal(t, "dead-lettered spans the storage rejected terminally", c.logs.All()[0].Message)
	assert.Equal(t, int64(2), c.deadLettered(t), "the counter reports the two dead-lettered spans")
}

func TestConsumeTraces_TransientPresentRetriesWithoutDeadLettering(t *testing.T) {
	sink := new(consumertest.TracesSink)
	td, ids := makeTraces()
	bulkErr := &esclient.BulkWriteError{
		Terminal:  []esclient.RejectedItem{rejected(ids[0])},
		Transient: true,
	}
	c := newTestConnector(t, directConfig(), sink, &fakeWriter{err: bulkErr})

	err := c.ConsumeTraces(context.Background(), td)
	require.ErrorIs(t, err, bulkErr, "a transient failure holds the offset for retry")
	// The poison is not dead-lettered yet: the batch will be retried, and it is
	// dead-lettered once a retry sees only terminal failures, so a retry storm never
	// re-emits the same span to the dead-letter sink.
	assert.Empty(t, sink.AllTraces())
}

func TestConsumeTraces_DeadLetterSinkFailureHoldsOffset(t *testing.T) {
	td, ids := makeTraces()
	bulkErr := &esclient.BulkWriteError{Terminal: []esclient.RejectedItem{rejected(ids[0])}}
	// The sink's exporter classifies a 4xx from its endpoint as permanent; the
	// connector must still return a retryable error, or its exporterhelper would
	// skip retries and the receiver would pause the partition.
	sinkErr := consumererror.NewPermanent(errors.New("dead-letter endpoint returned 401"))
	c := newTestConnector(t, directConfig(), consumertest.NewErr(sinkErr), &fakeWriter{err: bulkErr})

	err := c.ConsumeTraces(context.Background(), td)
	require.Error(t, err, "if the dead-letter sink rejects, hold the offset")
	require.ErrorContains(t, err, "dead-letter pipeline rejected 1 poison spans: Permanent error: dead-letter endpoint returned 401")
	assert.False(t, consumererror.IsPermanent(err), "a sink failure is retryable for the storage write")
}

func TestConsumeTraces_TransientBulkErrorNoTerminals(t *testing.T) {
	// drop mode returns a BulkWriteError with Transient=true and no Terminal items:
	// nothing to dead-letter, but the batch must still be retried.
	sink := new(consumertest.TracesSink)
	bulkErr := &esclient.BulkWriteError{Transient: true}
	c := newTestConnector(t, directConfig(), sink, &fakeWriter{err: bulkErr})
	td, _ := makeTraces()

	require.ErrorIs(t, c.ConsumeTraces(context.Background(), td), bulkErr)
	assert.Empty(t, sink.AllTraces(), "no terminal items means nothing is dead-lettered")
}

func TestConsumeTraces_UnmappedRejectionIsLoggedAndAdvances(t *testing.T) {
	sink := new(consumertest.TracesSink)
	// A service:operation document has no span _id, so it maps to no span.
	bulkErr := &esclient.BulkWriteError{Terminal: []esclient.RejectedItem{{Index: "jaeger-service", ID: "1234abcd", Status: 400}}}
	c := newTestConnector(t, directConfig(), sink, &fakeWriter{err: bulkErr})
	td, _ := makeTraces()

	require.NoError(t, c.ConsumeTraces(context.Background(), td), "a rejected lookup document must not stall the batch")
	assert.Empty(t, sink.AllTraces())
	require.Equal(t, 1, c.logs.Len())
	assert.Equal(t, "rejected documents that are not spans cannot be dead-lettered and were discarded", c.logs.All()[0].Message)
	assert.Equal(t, int64(1), c.logs.All()[0].ContextMap()["count"])
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
	bulkErr := &esclient.BulkWriteError{Terminal: []esclient.RejectedItem{rejected(ids[1])}}
	sink := new(consumertest.TracesSink)
	c := newTestConnector(t, cfg, sink, &fakeWriter{err: bulkErr})

	require.NoError(t, c.ConsumeTraces(context.Background(), td), "the caller is unblocked with the batch's verdict")
	require.Len(t, sink.AllTraces(), 1)
	assert.Equal(t, []string{"B"}, spanNames(sink.AllTraces()[0]))

	sentinel := errors.New("backend down")
	c2 := newTestConnector(t, cfg, sink, &fakeWriter{err: sentinel})
	require.ErrorIs(t, c2.ConsumeTraces(context.Background(), td), sentinel, "the caller sees the write's error, not an enqueue ack")
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

	out, unmapped, missing := filterPoisonSpans(td, []esclient.RejectedItem{rejected(idA), rejected(idC), {ID: "servicedoc"}})

	assert.Equal(t, 1, unmapped)
	assert.Empty(t, missing)
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
	out, unmapped, missing := filterPoisonSpans(td, []esclient.RejectedItem{{ID: "serviceoperationdoc"}})
	assert.Equal(t, 0, out.SpanCount())
	assert.Equal(t, 1, unmapped)
	assert.Empty(t, missing)
}

func TestConsumeTraces_RejectedIDWithoutSpanFailsTheBatch(t *testing.T) {
	sink := new(consumertest.TracesSink)
	td, _ := makeTraces()
	// A well-formed span _id that names no span in the batch: nothing can be
	// dead-lettered, and acknowledging would lose the span silently.
	stray := traceID(9).String() + "_" + spanID(9).String() + "_deadbeef"
	bulkErr := &esclient.BulkWriteError{Terminal: []esclient.RejectedItem{rejected(stray)}}
	c := newTestConnector(t, directConfig(), sink, &fakeWriter{err: bulkErr})

	err := c.ConsumeTraces(context.Background(), td)
	require.ErrorIs(t, err, bulkErr)
	require.ErrorContains(t, err, "1 rejected span documents match no span in the batch")
	require.ErrorContains(t, err, traceID(9).String()+"_"+spanID(9).String())
	assert.False(t, consumererror.IsPermanent(err), "the batch is retried, not dropped by the pipeline")
	assert.Empty(t, sink.AllTraces())
}

func TestConsumeTraces_RejectedItemWithoutIDFailsTheBatch(t *testing.T) {
	sink := new(consumertest.TracesSink)
	td, _ := makeTraces()
	// A bulk response item without an _id could be a span, so it is neither a
	// lookup document to discard nor a span to dead-letter: the batch must fail.
	bulkErr := &esclient.BulkWriteError{Terminal: []esclient.RejectedItem{{Index: "jaeger-span", Status: 400}}}
	c := newTestConnector(t, directConfig(), sink, &fakeWriter{err: bulkErr})

	err := c.ConsumeTraces(context.Background(), td)
	require.ErrorIs(t, err, bulkErr)
	require.ErrorContains(t, err, "(no _id in the bulk response)")
	assert.Empty(t, sink.AllTraces())
}

func TestFilterPoisonSpans_SharedSpanIDsAreBothReEmitted(t *testing.T) {
	// Two spans share trace and span id (a client and a server span); only one is
	// poison, but the prefix cannot tell them apart, so both are re-emitted.
	td := ptrace.NewTraces()
	ss := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
	_, id := makeSpan(ss, traceID(1), spanID(1), "client")
	makeSpan(ss, traceID(1), spanID(1), "server")
	makeSpan(ss, traceID(2), spanID(2), "other")

	out, unmapped, missing := filterPoisonSpans(td, []esclient.RejectedItem{rejected(id)})
	assert.Zero(t, unmapped)
	assert.Empty(t, missing)
	assert.ElementsMatch(t, []string{"client", "server"}, spanNames(out))
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
	err = conn.Start(context.Background(), hostWith("somestore", reportingFactory{Factory: factory, reports: true}))
	require.ErrorContains(t, err, "cannot create trace writer")
}

func TestStart_RefusesStorageThatDoesNotReportPoisonPills(t *testing.T) {
	tests := []struct {
		name    string
		factory tracestore.Factory
	}{
		{name: "factory without the capability", factory: writerFactory(&fakeWriter{})},
		{name: "factory that does not report (drop or async)", factory: reportingFactory{Factory: writerFactory(&fakeWriter{}), reports: false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn, err := createTracesToTraces(context.Background(), connectortest.NewNopSettings(componentType), directConfig(), consumertest.NewNop())
			require.NoError(t, err)
			err = conn.Start(context.Background(), hostWith("somestore", tt.factory))
			require.ErrorContains(t, err, `trace storage "somestore" does not report rejected spans`)
			require.ErrorContains(t, err, "poison_pill_handling: fail")
		})
	}
}

func TestStart_AcceptsReportingStorage(t *testing.T) {
	c := newTestConnector(t, directConfig(), consumertest.NewNop(), &fakeWriter{})
	assert.False(t, c.Capabilities().MutatesData)
}
