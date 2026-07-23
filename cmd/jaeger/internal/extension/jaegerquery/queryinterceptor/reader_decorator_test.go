// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package queryinterceptor

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/ptrace"

	pub "github.com/jaegertracing/jaeger/components/extension/jaegerquery/queryinterceptor"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// fakeReader is a minimal tracestore.Reader that records the query it received
// and yields a single configured batch (or error).
type fakeReader struct {
	gotQuery        tracestore.TraceQueryParams
	gotCtx          context.Context
	findCalled      bool
	batch           []ptrace.Traces
	ids             []tracestore.FoundTraceID
	services        []string
	err             error
	idsErr          error
	leadingErr      error
	summaryCalled   bool
	gotSummaryQuery tracestore.TraceQueryParams
	summaries       []tracestore.TraceSummary
	summaryErr      error
}

func (f *fakeReader) FindTraces(ctx context.Context, q tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
	f.findCalled = true
	f.gotQuery = q
	f.gotCtx = ctx
	return func(yield func([]ptrace.Traces, error) bool) {
		if f.err != nil {
			yield(nil, f.err)
			return
		}
		yield(f.batch, nil)
	}
}

func (f *fakeReader) GetTraces(_ context.Context, _ ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	return func(yield func([]ptrace.Traces, error) bool) {
		if f.leadingErr != nil {
			if !yield(nil, f.leadingErr) {
				return
			}
		}
		if f.err != nil {
			yield(nil, f.err)
			return
		}
		yield(f.batch, nil)
	}
}

func (f *fakeReader) FindTraceIDs(_ context.Context, q tracestore.TraceQueryParams) iter.Seq2[[]tracestore.FoundTraceID, error] {
	f.gotQuery = q
	return func(yield func([]tracestore.FoundTraceID, error) bool) {
		if f.idsErr != nil {
			yield(nil, f.idsErr)
			return
		}
		yield(f.ids, nil)
	}
}

func (f *fakeReader) GetServices(context.Context) ([]string, error) {
	return f.services, nil
}

func (*fakeReader) GetOperations(context.Context, tracestore.OperationQueryParams) ([]tracestore.Operation, error) {
	return []tracestore.Operation{{Name: "op"}}, nil
}

func (f *fakeReader) FindTraceSummaries(_ context.Context, q tracestore.TraceQueryParams) iter.Seq2[[]tracestore.TraceSummary, error] {
	f.summaryCalled = true
	f.gotSummaryQuery = q
	return func(yield func([]tracestore.TraceSummary, error) bool) {
		if f.summaryErr != nil {
			yield(nil, f.summaryErr)
			return
		}
		yield(f.summaries, nil)
	}
}

// fakeInterceptor lets each test supply the hook behavior it needs. It receives
// the public Query, exactly as a real interceptor would. The optional onQueryCtx
// and onResultCtx hooks transform (and observe) the context, so a test can assert
// how the decorator threads it from OnQuery into the reader and OnResult.
type fakeInterceptor struct {
	onQuery     func(pub.Query) (pub.Query, error)
	onResult    func([]ptrace.Traces) ([]ptrace.Traces, error)
	onQueryCtx  func(context.Context) context.Context
	onResultCtx func(context.Context) context.Context
}

func (f fakeInterceptor) OnQuery(ctx context.Context, q pub.Query) (context.Context, pub.Query, error) {
	if f.onQueryCtx != nil {
		ctx = f.onQueryCtx(ctx)
	}
	if f.onQuery != nil {
		nq, err := f.onQuery(q)
		return ctx, nq, err
	}
	return ctx, q, nil
}

func (f fakeInterceptor) OnResult(ctx context.Context, t []ptrace.Traces) (context.Context, []ptrace.Traces, error) {
	if f.onResultCtx != nil {
		ctx = f.onResultCtx(ctx)
	}
	if f.onResult != nil {
		nt, err := f.onResult(t)
		return ctx, nt, err
	}
	return ctx, t, nil
}

func tracesWith(key, val string) []ptrace.Traces {
	td := ptrace.NewTraces()
	span := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.Attributes().PutStr(key, val)
	return []ptrace.Traces{td}
}

func firstSpanAttr(t *testing.T, batch []ptrace.Traces, key string) string {
	t.Helper()
	require.NotEmpty(t, batch)
	attrs := batch[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	v, ok := attrs.Get(key)
	require.True(t, ok, "attribute %q not present", key)
	return v.Str()
}

func collectTraces(it iter.Seq2[[]ptrace.Traces, error]) ([][]ptrace.Traces, error) {
	var out [][]ptrace.Traces
	for batch, err := range it {
		if err != nil {
			return out, err
		}
		out = append(out, batch)
	}
	return out, nil
}

func redactResult(key string) func([]ptrace.Traces) ([]ptrace.Traces, error) {
	return func(batch []ptrace.Traces) ([]ptrace.Traces, error) {
		for _, td := range batch {
			rss := td.ResourceSpans()
			for i := 0; i < rss.Len(); i++ {
				sss := rss.At(i).ScopeSpans()
				for j := 0; j < sss.Len(); j++ {
					spans := sss.At(j).Spans()
					for k := 0; k < spans.Len(); k++ {
						if _, ok := spans.At(k).Attributes().Get(key); ok {
							spans.At(k).Attributes().PutStr(key, "REDACTED")
						}
					}
				}
			}
		}
		return batch, nil
	}
}

func TestReader_FindTraces_AppliesQueryAndResultHooks(t *testing.T) {
	next := &fakeReader{batch: tracesWith("secret", "value")}
	ic := fakeInterceptor{
		onQuery: func(q pub.Query) (pub.Query, error) {
			q.ServiceName = "gated"
			return q, nil
		},
		onResult: redactResult("secret"),
	}
	r := NewReaderDecorator(next, ic)

	out, err := collectTraces(r.FindTraces(t.Context(), tracestore.TraceQueryParams{ServiceName: "original"}))
	require.NoError(t, err)
	assert.Equal(t, "gated", next.gotQuery.ServiceName, "pre-query hook must reach storage")
	require.Len(t, out, 1)
	assert.Equal(t, "REDACTED", firstSpanAttr(t, out[0], "secret"), "result hook must redact")
}

func TestReader_FindTraces_QueryRejectionSkipsStorage(t *testing.T) {
	sentinel := errors.New("denied")
	next := &fakeReader{batch: tracesWith("k", "v")}
	r := NewReaderDecorator(next, fakeInterceptor{
		onQuery: func(q pub.Query) (pub.Query, error) { return q, sentinel },
	})

	_, err := collectTraces(r.FindTraces(t.Context(), tracestore.TraceQueryParams{}))
	require.ErrorIs(t, err, sentinel)
	assert.False(t, next.findCalled, "storage must not be queried when the query is rejected")
}

func TestReader_FindTraces_ResultErrorAborts(t *testing.T) {
	sentinel := errors.New("sanitize failed")
	next := &fakeReader{batch: tracesWith("k", "v")}
	r := NewReaderDecorator(next, fakeInterceptor{
		onResult: func([]ptrace.Traces) ([]ptrace.Traces, error) { return nil, sentinel },
	})

	_, err := collectTraces(r.FindTraces(t.Context(), tracestore.TraceQueryParams{}))
	require.ErrorIs(t, err, sentinel)
}

func TestReader_FindTraces_StorageErrorPropagates(t *testing.T) {
	sentinel := errors.New("storage down")
	next := &fakeReader{err: sentinel}
	r := NewReaderDecorator(next, fakeInterceptor{})

	_, err := collectTraces(r.FindTraces(t.Context(), tracestore.TraceQueryParams{}))
	require.ErrorIs(t, err, sentinel)
}

func TestReader_GetTraces_AppliesResultHook(t *testing.T) {
	next := &fakeReader{batch: tracesWith("secret", "value")}
	r := NewReaderDecorator(next, fakeInterceptor{onResult: redactResult("secret")})

	out, err := collectTraces(r.GetTraces(t.Context(), tracestore.GetTraceParams{}))
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "REDACTED", firstSpanAttr(t, out[0], "secret"))
}

func TestReader_GetTraces_StorageErrorPropagates(t *testing.T) {
	sentinel := errors.New("storage down")
	r := NewReaderDecorator(&fakeReader{err: sentinel}, fakeInterceptor{})
	_, err := collectTraces(r.GetTraces(t.Context(), tracestore.GetTraceParams{}))
	require.ErrorIs(t, err, sentinel)
}

func TestReader_GetTraces_ResultErrorAborts(t *testing.T) {
	sentinel := errors.New("sanitize failed")
	next := &fakeReader{batch: tracesWith("k", "v")}
	r := NewReaderDecorator(next, fakeInterceptor{
		onResult: func([]ptrace.Traces) ([]ptrace.Traces, error) { return nil, sentinel },
	})
	_, err := collectTraces(r.GetTraces(t.Context(), tracestore.GetTraceParams{}))
	require.ErrorIs(t, err, sentinel)
}

func TestReader_GetTraces_ContinuesAfterError(t *testing.T) {
	sentinel := errors.New("transient")
	next := &fakeReader{leadingErr: sentinel, batch: tracesWith("secret", "value")}
	r := NewReaderDecorator(next, fakeInterceptor{onResult: redactResult("secret")})

	var errs, batches int
	for batch, err := range r.GetTraces(t.Context(), tracestore.GetTraceParams{}) {
		if err != nil {
			require.ErrorIs(t, err, sentinel)
			errs++
			continue
		}
		assert.Equal(t, "REDACTED", firstSpanAttr(t, batch, "secret"))
		batches++
	}
	assert.Equal(t, 1, errs)
	assert.Equal(t, 1, batches)
}

// multiBatchReader yields a fixed sequence of trace batches, so a test can
// assert what the decorator does with batches after the first.
type multiBatchReader struct {
	*fakeReader
	batches [][]ptrace.Traces
}

func (r *multiBatchReader) FindTraces(context.Context, tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
	return r.yieldBatches
}

func (r *multiBatchReader) GetTraces(context.Context, ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
	return r.yieldBatches
}

func (r *multiBatchReader) yieldBatches(yield func([]ptrace.Traces, error) bool) {
	for _, b := range r.batches {
		if !yield(b, nil) {
			return
		}
	}
}

// assertResultErrorStops verifies that an OnResult failure aborts the stream:
// even a consumer that keeps ranging after the error must never receive a later
// batch, and OnResult must not run again. This guards the redaction/authorization
// use case, where emitting a later batch after a failed sanitize would leak data.
func assertResultErrorStops(t *testing.T, call func(tracestore.Reader) iter.Seq2[[]ptrace.Traces, error]) {
	sentinel := errors.New("sanitize failed")
	next := &multiBatchReader{
		fakeReader: &fakeReader{},
		batches:    [][]ptrace.Traces{tracesWith("k", "1"), tracesWith("k", "2")},
	}
	onResultCalls := 0
	r := NewReaderDecorator(next, fakeInterceptor{
		onResult: func([]ptrace.Traces) ([]ptrace.Traces, error) {
			onResultCalls++
			return nil, sentinel
		},
	})

	var errs, batches int
	for _, err := range call(r) {
		if err != nil {
			require.ErrorIs(t, err, sentinel)
			errs++
			continue
		}
		batches++
	}
	assert.Equal(t, 1, errs, "exactly one error, then the stream aborts")
	assert.Zero(t, batches, "no batch may be delivered after an OnResult error")
	assert.Equal(t, 1, onResultCalls, "OnResult must not run on batches after it fails")
}

func TestReader_FindTraces_ResultErrorStopsIteration(t *testing.T) {
	assertResultErrorStops(t, func(r tracestore.Reader) iter.Seq2[[]ptrace.Traces, error] {
		return r.FindTraces(t.Context(), tracestore.TraceQueryParams{})
	})
}

func TestReader_GetTraces_ResultErrorStopsIteration(t *testing.T) {
	assertResultErrorStops(t, func(r tracestore.Reader) iter.Seq2[[]ptrace.Traces, error] {
		return r.GetTraces(t.Context(), tracestore.GetTraceParams{})
	})
}

func TestReader_FindTraceIDs_AppliesQueryHook(t *testing.T) {
	next := &fakeReader{ids: []tracestore.FoundTraceID{{}}}
	r := NewReaderDecorator(next, fakeInterceptor{
		onQuery: func(q pub.Query) (pub.Query, error) {
			q.ServiceName = "gated"
			return q, nil
		},
	})

	var got [][]tracestore.FoundTraceID
	for ids, err := range r.FindTraceIDs(t.Context(), tracestore.TraceQueryParams{}) {
		require.NoError(t, err)
		got = append(got, ids)
	}
	assert.Equal(t, "gated", next.gotQuery.ServiceName)
	assert.Len(t, got, 1)
}

func TestReader_FindTraceIDs_QueryRejectionSkipsStorage(t *testing.T) {
	sentinel := errors.New("denied")
	next := &fakeReader{ids: []tracestore.FoundTraceID{{}}}
	r := NewReaderDecorator(next, fakeInterceptor{
		onQuery: func(q pub.Query) (pub.Query, error) { return q, sentinel },
	})
	var err error
	for _, e := range r.FindTraceIDs(t.Context(), tracestore.TraceQueryParams{}) {
		err = e
	}
	require.ErrorIs(t, err, sentinel)
}

func TestReader_FindTraceIDs_StorageErrorPropagates(t *testing.T) {
	sentinel := errors.New("storage down")
	r := NewReaderDecorator(&fakeReader{idsErr: sentinel}, fakeInterceptor{})
	var err error
	for _, e := range r.FindTraceIDs(t.Context(), tracestore.TraceQueryParams{}) {
		err = e
	}
	require.ErrorIs(t, err, sentinel)
}

// The early-stop tests exercise the "consumer stopped iterating" branches: when
// the range loop breaks, yield returns false and the decorator must return.
func TestReader_EarlyStop(t *testing.T) {
	next := &fakeReader{batch: tracesWith("k", "v"), ids: []tracestore.FoundTraceID{{}}}
	r := NewReaderDecorator(next, fakeInterceptor{})

	for range r.FindTraces(t.Context(), tracestore.TraceQueryParams{}) {
		break
	}
	for range r.GetTraces(t.Context(), tracestore.GetTraceParams{}) {
		break
	}
	for range r.FindTraceIDs(t.Context(), tracestore.TraceQueryParams{}) {
		break
	}
}

func TestReader_PassThroughMethods(t *testing.T) {
	next := &fakeReader{services: []string{"svc"}}
	r := NewReaderDecorator(next, fakeInterceptor{})

	svcs, err := r.GetServices(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"svc"}, svcs)

	ops, err := r.GetOperations(t.Context(), tracestore.OperationQueryParams{})
	require.NoError(t, err)
	assert.Equal(t, []tracestore.Operation{{Name: "op"}}, ops)
}

func TestReader_ChainAppliesInOrder(t *testing.T) {
	next := &fakeReader{batch: tracesWith("v", "0")}
	var order []string
	first := fakeInterceptor{onQuery: func(q pub.Query) (pub.Query, error) {
		order = append(order, "first")
		return q, nil
	}}
	second := fakeInterceptor{onQuery: func(q pub.Query) (pub.Query, error) {
		order = append(order, "second")
		return q, nil
	}}
	r := NewReaderDecorator(next, first, second)

	_, err := collectTraces(r.FindTraces(t.Context(), tracestore.TraceQueryParams{}))
	require.NoError(t, err)
	assert.Equal(t, []string{"first", "second"}, order)
}

type ctxKey struct{}

func TestReader_FindTraces_ThreadsQueryContextToStorageAndResult(t *testing.T) {
	next := &fakeReader{batch: tracesWith("k", "v")}
	var resultSaw any
	ic := fakeInterceptor{
		onQueryCtx: func(ctx context.Context) context.Context {
			return context.WithValue(ctx, ctxKey{}, "from-onquery")
		},
		onResultCtx: func(ctx context.Context) context.Context {
			resultSaw = ctx.Value(ctxKey{})
			return ctx
		},
	}
	r := NewReaderDecorator(next, ic)

	_, err := collectTraces(r.FindTraces(t.Context(), tracestore.TraceQueryParams{}))
	require.NoError(t, err)
	assert.Equal(t, "from-onquery", next.gotCtx.Value(ctxKey{}), "storage reader must see the context OnQuery returned")
	assert.Equal(t, "from-onquery", resultSaw, "OnResult must see the context OnQuery returned")
}

func TestReader_FindTraces_ThreadsResultContextAcrossBatches(t *testing.T) {
	next := &multiBatchReader{
		fakeReader: &fakeReader{},
		batches:    [][]ptrace.Traces{tracesWith("k", "1"), tracesWith("k", "2")},
	}
	var seen []int
	ic := fakeInterceptor{
		onResultCtx: func(ctx context.Context) context.Context {
			n, _ := ctx.Value(ctxKey{}).(int)
			seen = append(seen, n)
			return context.WithValue(ctx, ctxKey{}, n+1)
		},
	}
	r := NewReaderDecorator(next, ic)

	_, err := collectTraces(r.FindTraces(t.Context(), tracestore.TraceQueryParams{}))
	require.NoError(t, err)
	assert.Equal(t, []int{0, 1}, seen, "OnResult's returned context must thread into the next batch")
}

func TestReader_GetTraces_ThreadsResultContextAcrossBatches(t *testing.T) {
	next := &multiBatchReader{
		fakeReader: &fakeReader{},
		batches:    [][]ptrace.Traces{tracesWith("k", "1"), tracesWith("k", "2")},
	}
	var seen []int
	ic := fakeInterceptor{
		onResultCtx: func(ctx context.Context) context.Context {
			n, _ := ctx.Value(ctxKey{}).(int)
			seen = append(seen, n)
			return context.WithValue(ctx, ctxKey{}, n+1)
		},
	}
	r := NewReaderDecorator(next, ic)

	_, err := collectTraces(r.GetTraces(t.Context(), tracestore.GetTraceParams{}))
	require.NoError(t, err)
	assert.Equal(t, []int{0, 1}, seen, "OnResult's returned context must thread into the next batch")
}

func TestReader_FindTraceSummaries_AppliesQueryHook(t *testing.T) {
	next := &fakeReader{summaries: []tracestore.TraceSummary{{RootServiceName: "svc"}}}
	r := NewReaderDecorator(next, fakeInterceptor{
		onQuery: func(q pub.Query) (pub.Query, error) {
			q.ServiceName = "gated"
			return q, nil
		},
	})

	var got [][]tracestore.TraceSummary
	for s, err := range r.FindTraceSummaries(t.Context(), tracestore.TraceQueryParams{ServiceName: "original"}) {
		require.NoError(t, err)
		got = append(got, s)
	}
	assert.Equal(t, "gated", next.gotSummaryQuery.ServiceName, "pre-query hook must reach storage")
	require.Len(t, got, 1)
	require.Len(t, got[0], 1)
	assert.Equal(t, "svc", got[0][0].RootServiceName)
}

func TestReader_FindTraceSummaries_QueryRejectionSkipsStorage(t *testing.T) {
	sentinel := errors.New("denied")
	next := &fakeReader{summaries: []tracestore.TraceSummary{{}}}
	r := NewReaderDecorator(next, fakeInterceptor{
		onQuery: func(q pub.Query) (pub.Query, error) { return q, sentinel },
	})

	var err error
	for _, e := range r.FindTraceSummaries(t.Context(), tracestore.TraceQueryParams{}) {
		err = e
	}
	require.ErrorIs(t, err, sentinel)
	assert.False(t, next.summaryCalled, "storage must not be queried when the query is rejected")
}

func TestReader_FindTraceSummaries_StorageErrorPropagates(t *testing.T) {
	sentinel := errors.New("storage down")
	next := &fakeReader{summaryErr: sentinel}
	r := NewReaderDecorator(next, fakeInterceptor{})

	var err error
	for _, e := range r.FindTraceSummaries(t.Context(), tracestore.TraceQueryParams{}) {
		err = e
	}
	require.ErrorIs(t, err, sentinel)
}
