// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"context"
	"fmt"
	"iter"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/adjuster"
	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	depstoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
)

const millisToNanosMultiplier = int64(time.Millisecond / time.Nanosecond)

var (
	defaultDependencyLookbackDuration = time.Hour * 24

	testTraceID = pcommon.TraceID([16]byte{1})
)

type testQueryService struct {
	queryService *QueryService
	traceReader  *tracestoremocks.Reader
	depsReader   *depstoremocks.Reader

	archiveTraceReader *tracestoremocks.Reader
	archiveTraceWriter *tracestoremocks.Writer
}

type testOption func(*testQueryService, *QueryServiceOptions)

func withArchiveTraceReader() testOption {
	return func(tqs *testQueryService, options *QueryServiceOptions) {
		r := &tracestoremocks.Reader{}
		tqs.archiveTraceReader = r
		options.ArchiveTraceReader = r
	}
}

func withArchiveTraceWriter() testOption {
	return func(tqs *testQueryService, options *QueryServiceOptions) {
		r := &tracestoremocks.Writer{}
		tqs.archiveTraceWriter = r
		options.ArchiveTraceWriter = r
	}
}

func initializeTestService(opts ...testOption) *testQueryService {
	traceReader := &tracestoremocks.Reader{}
	dependencyStorage := &depstoremocks.Reader{}

	options := QueryServiceOptions{}

	tqs := testQueryService{
		traceReader: traceReader,
		depsReader:  dependencyStorage,
	}

	for _, opt := range opts {
		opt(&tqs, &options)
	}

	tqs.queryService = NewQueryService(traceReader, dependencyStorage, options)
	return &tqs
}

func makeTestTrace() ptrace.Traces {
	trace := ptrace.NewTraces()
	resources := trace.ResourceSpans().AppendEmpty()
	scopes := resources.ScopeSpans().AppendEmpty()

	spanA := scopes.Spans().AppendEmpty()
	spanA.SetTraceID(testTraceID)
	spanA.SetSpanID(pcommon.SpanID([8]byte{1}))

	spanB := scopes.Spans().AppendEmpty()
	spanB.SetTraceID(testTraceID)
	spanB.SetSpanID(pcommon.SpanID([8]byte{2}))

	return trace
}

func TestGetTraces_ErrorInReader(t *testing.T) {
	tqs := initializeTestService()
	tqs.traceReader.On("GetTraces", mock.Anything, []tracestore.GetTraceParams{{TraceID: testTraceID}}).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield(nil, assert.AnError)
		})).Once()

	params := GetTraceParams{
		TraceIDs: []tracestore.GetTraceParams{
			{
				TraceID: testTraceID,
			},
		},
	}
	getTracesIter := tqs.queryService.GetTraces(context.Background(), params)
	_, err := jiter.FlattenWithErrors(getTracesIter)
	require.ErrorIs(t, err, assert.AnError)
}

func TestGetTraces_Success(t *testing.T) {
	tqs := initializeTestService()
	params := GetTraceParams{
		TraceIDs: []tracestore.GetTraceParams{{TraceID: testTraceID}},
	}
	tqs.traceReader.On("GetTraces", mock.Anything, params.TraceIDs).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{makeTestTrace()}, nil)
		})).Once()

	getTracesIter := tqs.queryService.GetTraces(context.Background(), params)
	gotTraces, err := jiter.FlattenWithErrors(getTracesIter)
	require.NoError(t, err)
	require.Len(t, gotTraces, 1)

	gotSpans := gotTraces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans()
	require.Equal(t, 2, gotSpans.Len())
	require.Equal(t, testTraceID, gotSpans.At(0).TraceID())
	require.EqualValues(t, [8]byte{1}, gotSpans.At(0).SpanID())
	require.Equal(t, testTraceID, gotSpans.At(1).TraceID())
	require.EqualValues(t, [8]byte{2}, gotSpans.At(1).SpanID())
}

func TestGetTraces_WithRawTraces(t *testing.T) {
	tests := []struct {
		rawTraces  bool
		attributes pcommon.Map
		expected   pcommon.Map
	}{
		{
			// tags should not get sorted by SortCollections adjuster
			rawTraces: true,
			attributes: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutStr("z", "key")
				m.PutStr("a", "key")
				return m
			}(),
			expected: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutStr("z", "key")
				m.PutStr("a", "key")
				return m
			}(),
		},
		{
			// tags should get sorted by SortCollections adjuster
			rawTraces: false,
			attributes: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutStr("z", "key")
				m.PutStr("a", "key")
				return m
			}(),
			expected: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutStr("a", "key")
				m.PutStr("z", "key")
				return m
			}(),
		},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("rawTraces=%v", test.rawTraces), func(t *testing.T) {
			trace := makeTestTrace()
			test.attributes.CopyTo(trace.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes())
			responseIter := iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{trace}, nil)
			})

			tqs := initializeTestService()
			params := GetTraceParams{
				TraceIDs:  []tracestore.GetTraceParams{{TraceID: testTraceID}},
				RawTraces: test.rawTraces,
			}
			tqs.traceReader.On("GetTraces", mock.Anything, params.TraceIDs).
				Return(responseIter).Once()

			getTracesIter := tqs.queryService.GetTraces(context.Background(), params)
			gotTraces, err := jiter.FlattenWithErrors(getTracesIter)
			require.NoError(t, err)

			require.Len(t, gotTraces, 1)
			gotAttributes := gotTraces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
			require.Equal(t, test.expected, gotAttributes)
		})
	}
}

func TestGetTraces_TraceInArchiveStorage(t *testing.T) {
	tqs := initializeTestService(withArchiveTraceReader())

	params := GetTraceParams{
		TraceIDs: []tracestore.GetTraceParams{{TraceID: testTraceID}},
	}

	tqs.traceReader.On("GetTraces", mock.Anything, params.TraceIDs).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{}, nil)
		})).Once()

	tqs.archiveTraceReader.On("GetTraces", mock.Anything, params.TraceIDs).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{makeTestTrace()}, nil)
		})).Once()

	getTracesIter := tqs.queryService.GetTraces(context.Background(), params)
	gotTraces, err := jiter.FlattenWithErrors(getTracesIter)
	require.NoError(t, err)
	require.Len(t, gotTraces, 1)

	gotSpans := gotTraces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans()
	require.Equal(t, 2, gotSpans.Len())
	require.Equal(t, testTraceID, gotSpans.At(0).TraceID())
	require.EqualValues(t, [8]byte{1}, gotSpans.At(0).SpanID())
	require.Equal(t, testTraceID, gotSpans.At(1).TraceID())
	require.EqualValues(t, [8]byte{2}, gotSpans.At(1).SpanID())
}

func TestGetTraces_SplitTraceInArchiveStorage(t *testing.T) {
	tqs := initializeTestService(withArchiveTraceReader())

	params := GetTraceParams{
		TraceIDs: []tracestore.GetTraceParams{{TraceID: testTraceID}},
	}

	// Primary reader returns no traces
	tqs.traceReader.On("GetTraces", mock.Anything, params.TraceIDs).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{}, nil)
		})).Once()

	// Archive reader returns the trace split across two chunks
	tqs.archiveTraceReader.On("GetTraces", mock.Anything, params.TraceIDs).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			// First chunk with span 1
			traceChunk1 := ptrace.NewTraces()
			resources1 := traceChunk1.ResourceSpans().AppendEmpty()
			scopes1 := resources1.ScopeSpans().AppendEmpty()
			span1 := scopes1.Spans().AppendEmpty()
			span1.SetTraceID(testTraceID)
			span1.SetSpanID(pcommon.SpanID([8]byte{1}))
			span1.SetName("span1")

			// Second chunk with span 2 (same trace ID)
			traceChunk2 := ptrace.NewTraces()
			resources2 := traceChunk2.ResourceSpans().AppendEmpty()
			scopes2 := resources2.ScopeSpans().AppendEmpty()
			span2 := scopes2.Spans().AppendEmpty()
			span2.SetTraceID(testTraceID)
			span2.SetSpanID(pcommon.SpanID([8]byte{2}))
			span2.SetName("span2")

			// Yield both chunks in the same batch
			yield([]ptrace.Traces{traceChunk1, traceChunk2}, nil)
		})).Once()

	getTracesIter := tqs.queryService.GetTraces(context.Background(), params)
	gotTraces, err := jiter.FlattenWithErrors(getTracesIter)
	require.NoError(t, err)
	require.Len(t, gotTraces, 1, "expected one aggregated trace")

	// Verify the trace was properly aggregated
	require.Equal(t, 2, gotTraces[0].ResourceSpans().Len(), "expected 2 resource spans after aggregation")

	gotSpan1 := gotTraces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	require.Equal(t, testTraceID, gotSpan1.TraceID())
	require.EqualValues(t, [8]byte{1}, gotSpan1.SpanID())
	require.Equal(t, "span1", gotSpan1.Name())

	gotSpan2 := gotTraces[0].ResourceSpans().At(1).ScopeSpans().At(0).Spans().At(0)
	require.Equal(t, testTraceID, gotSpan2.TraceID())
	require.EqualValues(t, [8]byte{2}, gotSpan2.SpanID())
	require.Equal(t, "span2", gotSpan2.Name())
}

func TestGetTraces_SplitTraceInArchiveStorageWithRawTraces(t *testing.T) {
	tqs := initializeTestService(withArchiveTraceReader())

	params := GetTraceParams{
		TraceIDs:  []tracestore.GetTraceParams{{TraceID: testTraceID}},
		RawTraces: true, // Request raw traces without aggregation
	}

	// Primary reader returns no traces
	tqs.traceReader.On("GetTraces", mock.Anything, params.TraceIDs).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			yield([]ptrace.Traces{}, nil)
		})).Once()

	// Archive reader returns the trace split across two chunks
	tqs.archiveTraceReader.On("GetTraces", mock.Anything, params.TraceIDs).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			// First chunk with span 1
			traceChunk1 := ptrace.NewTraces()
			resources1 := traceChunk1.ResourceSpans().AppendEmpty()
			scopes1 := resources1.ScopeSpans().AppendEmpty()
			span1 := scopes1.Spans().AppendEmpty()
			span1.SetTraceID(testTraceID)
			span1.SetSpanID(pcommon.SpanID([8]byte{1}))
			span1.SetName("span1")

			// Second chunk with span 2 (same trace ID)
			traceChunk2 := ptrace.NewTraces()
			resources2 := traceChunk2.ResourceSpans().AppendEmpty()
			scopes2 := resources2.ScopeSpans().AppendEmpty()
			span2 := scopes2.Spans().AppendEmpty()
			span2.SetTraceID(testTraceID)
			span2.SetSpanID(pcommon.SpanID([8]byte{2}))
			span2.SetName("span2")

			// Yield both chunks in the same batch
			yield([]ptrace.Traces{traceChunk1, traceChunk2}, nil)
		})).Once()

	getTracesIter := tqs.queryService.GetTraces(context.Background(), params)
	gotTraces, err := jiter.FlattenWithErrors(getTracesIter)
	require.NoError(t, err)
	require.Len(t, gotTraces, 2, "expected two separate trace chunks with RawTraces=true")

	// Verify chunks are NOT aggregated when RawTraces is true
	gotSpan1 := gotTraces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	require.Equal(t, testTraceID, gotSpan1.TraceID())
	require.EqualValues(t, [8]byte{1}, gotSpan1.SpanID())
	require.Equal(t, "span1", gotSpan1.Name())

	gotSpan2 := gotTraces[1].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	require.Equal(t, testTraceID, gotSpan2.TraceID())
	require.EqualValues(t, [8]byte{2}, gotSpan2.SpanID())
	require.Equal(t, "span2", gotSpan2.Name())
}

func TestGetServices(t *testing.T) {
	tqs := initializeTestService()
	expected := []string{"trifle", "bling"}
	tqs.traceReader.On("GetServices", mock.Anything).Return(expected, nil).Once()

	actualServices, err := tqs.queryService.GetServices(context.Background())
	require.NoError(t, err)
	assert.Equal(t, expected, actualServices)
}

func TestGetOperations(t *testing.T) {
	tqs := initializeTestService()
	expected := []tracestore.Operation{{Name: "", SpanKind: ""}, {Name: "get", SpanKind: ""}}
	operationQuery := tracestore.OperationQueryParams{ServiceName: "abc/trifle"}
	tqs.traceReader.On(
		"GetOperations",
		mock.Anything,
		operationQuery,
	).Return(expected, nil).Once()

	actualOperations, err := tqs.queryService.GetOperations(context.Background(), operationQuery)
	require.NoError(t, err)
	assert.Equal(t, expected, actualOperations)
}

func TestFindTraces_Success(t *testing.T) {
	tqs := initializeTestService()
	responseIter := iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
		yield([]ptrace.Traces{makeTestTrace()}, nil)
	})

	duration := 20 * time.Millisecond
	now := time.Now()
	queryParams := tracestore.TraceQueryParams{
		ServiceName:   "service",
		OperationName: "operation",
		StartTimeMax:  now,
		DurationMin:   duration,
		SearchDepth:   200,
	}
	tqs.traceReader.On("FindTraces", mock.Anything, queryParams).Return(responseIter).Once()

	query := TraceQueryParams{TraceQueryParams: queryParams}
	getTracesIter := tqs.queryService.FindTraces(context.Background(), query)
	gotTraces, err := jiter.FlattenWithErrors(getTracesIter)
	require.NoError(t, err)
	require.Len(t, gotTraces, 1)

	gotSpans := gotTraces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans()
	require.Equal(t, 2, gotSpans.Len())
	require.Equal(t, testTraceID, gotSpans.At(0).TraceID())
	require.EqualValues(t, [8]byte{1}, gotSpans.At(0).SpanID())
	require.Equal(t, testTraceID, gotSpans.At(1).TraceID())
	require.EqualValues(t, [8]byte{2}, gotSpans.At(1).SpanID())
}

func TestFindTraces_WithRawTraces_PerformsAdjustment(t *testing.T) {
	tests := []struct {
		rawTraces  bool
		attributes pcommon.Map
		expected   pcommon.Map
	}{
		{
			// tags should not get sorted by SortTagsAndLogFields adjuster
			rawTraces: true,
			attributes: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutStr("z", "key")
				m.PutStr("a", "key")
				return m
			}(),
			expected: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutStr("z", "key")
				m.PutStr("a", "key")
				return m
			}(),
		},
		{
			// tags should get sorted by SortTagsAndLogFields adjuster
			rawTraces: false,
			attributes: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutStr("z", "key")
				m.PutStr("a", "key")
				return m
			}(),
			expected: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutStr("a", "key")
				m.PutStr("z", "key")
				return m
			}(),
		},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("rawTraces=%v", test.rawTraces), func(t *testing.T) {
			trace := makeTestTrace()
			test.attributes.CopyTo(trace.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes())
			responseIter := iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{trace}, nil)
			})

			tqs := initializeTestService()
			duration, err := time.ParseDuration("20ms")
			require.NoError(t, err)
			now := time.Now()
			tqs.traceReader.On("FindTraces", mock.Anything, tracestore.TraceQueryParams{
				ServiceName:   "service",
				OperationName: "operation",
				StartTimeMax:  now,
				DurationMin:   duration,
				SearchDepth:   200,
			}).
				Return(responseIter).Once()

			query := TraceQueryParams{
				TraceQueryParams: tracestore.TraceQueryParams{
					ServiceName:   "service",
					OperationName: "operation",
					StartTimeMax:  now,
					DurationMin:   duration,
					SearchDepth:   200,
				},
				RawTraces: test.rawTraces,
			}
			getTracesIter := tqs.queryService.FindTraces(context.Background(), query)
			gotTraces, err := jiter.FlattenWithErrors(getTracesIter)
			require.NoError(t, err)

			require.Len(t, gotTraces, 1)
			gotAttributes := gotTraces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
			require.Equal(t, test.expected, gotAttributes)
		})
	}
}

func TestFindTraces_WithRawTraces_PerformsAggregation(t *testing.T) {
	tests := []struct {
		rawTraces           bool
		traces              []ptrace.Traces
		expected            []ptrace.Traces
		expectedAdjustCalls int
	}{
		{
			rawTraces: true,
			traces: func() []ptrace.Traces {
				traceA := ptrace.NewTraces()
				resourcesA := traceA.ResourceSpans().AppendEmpty()
				scopesA := resourcesA.ScopeSpans().AppendEmpty()
				spanA := scopesA.Spans().AppendEmpty()
				spanA.SetTraceID(testTraceID)
				spanA.SetName("spanA")
				spanA.SetSpanID(pcommon.SpanID([8]byte{1}))

				traceB := ptrace.NewTraces()
				resourcesB := traceB.ResourceSpans().AppendEmpty()
				scopesB := resourcesB.ScopeSpans().AppendEmpty()
				spanB := scopesB.Spans().AppendEmpty()
				spanB.SetTraceID(testTraceID)
				spanB.SetName("spanB")
				spanB.SetSpanID(pcommon.SpanID([8]byte{2}))

				return []ptrace.Traces{traceA, traceB}
			}(),
			expected: func() []ptrace.Traces {
				traceA := ptrace.NewTraces()
				resourcesA := traceA.ResourceSpans().AppendEmpty()
				scopesA := resourcesA.ScopeSpans().AppendEmpty()
				spanA := scopesA.Spans().AppendEmpty()
				spanA.SetTraceID(testTraceID)
				spanA.SetName("spanA")
				spanA.SetSpanID(pcommon.SpanID([8]byte{1}))

				traceB := ptrace.NewTraces()
				resourcesB := traceB.ResourceSpans().AppendEmpty()
				scopesB := resourcesB.ScopeSpans().AppendEmpty()
				spanB := scopesB.Spans().AppendEmpty()
				spanB.SetTraceID(testTraceID)
				spanB.SetName("spanB")
				spanB.SetSpanID(pcommon.SpanID([8]byte{2}))

				return []ptrace.Traces{traceA, traceB}
			}(),
			expectedAdjustCalls: 0,
		},
		{
			rawTraces: false,
			traces: func() []ptrace.Traces {
				traceA := ptrace.NewTraces()
				resourcesA := traceA.ResourceSpans().AppendEmpty()
				scopesA := resourcesA.ScopeSpans().AppendEmpty()
				spanA := scopesA.Spans().AppendEmpty()
				spanA.SetTraceID(testTraceID)
				spanA.SetSpanID(pcommon.SpanID([8]byte{1}))

				traceB := ptrace.NewTraces()
				resourcesB := traceB.ResourceSpans().AppendEmpty()
				scopesB := resourcesB.ScopeSpans().AppendEmpty()
				spanB := scopesB.Spans().AppendEmpty()
				spanB.SetTraceID(testTraceID)
				spanB.SetSpanID(pcommon.SpanID([8]byte{2}))

				return []ptrace.Traces{traceA, traceB}
			}(),
			expected: func() []ptrace.Traces {
				traceA := ptrace.NewTraces()
				resourcesA := traceA.ResourceSpans().AppendEmpty()
				scopesA := resourcesA.ScopeSpans().AppendEmpty()
				spanA := scopesA.Spans().AppendEmpty()
				spanA.SetTraceID(testTraceID)
				spanA.SetSpanID(pcommon.SpanID([8]byte{1}))

				resourcesB := ptrace.NewResourceSpans()
				scopesB := resourcesB.ScopeSpans().AppendEmpty()
				spanB := scopesB.Spans().AppendEmpty()
				spanB.SetTraceID(testTraceID)
				spanB.SetSpanID(pcommon.SpanID([8]byte{2}))

				resourcesB.CopyTo(traceA.ResourceSpans().AppendEmpty())

				return []ptrace.Traces{traceA}
			}(),
			// even though there are 2 input chunks, they are for the same trace,
			// so we expect only 1 call to Adjuster.
			expectedAdjustCalls: 1,
		},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("rawTraces=%v", test.rawTraces), func(t *testing.T) {
			responseIter := iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
				yield(test.traces, nil)
			})
			adjustCalls := 0
			adj := adjuster.Func(func(_ ptrace.Traces) {
				adjustCalls++
			})

			tqs := initializeTestService()
			tqs.queryService.adjuster = adj
			duration, err := time.ParseDuration("20ms")
			require.NoError(t, err)
			now := time.Now()
			tqs.traceReader.On("FindTraces", mock.Anything, tracestore.TraceQueryParams{
				ServiceName:   "service",
				OperationName: "operation",
				StartTimeMax:  now,
				DurationMin:   duration,
				SearchDepth:   200,
			}).
				Return(responseIter).Once()

			query := TraceQueryParams{
				TraceQueryParams: tracestore.TraceQueryParams{
					ServiceName:   "service",
					OperationName: "operation",
					StartTimeMax:  now,
					DurationMin:   duration,
					SearchDepth:   200,
				},
				RawTraces: test.rawTraces,
			}
			getTracesIter := tqs.queryService.FindTraces(context.Background(), query)
			gotTraces, err := jiter.FlattenWithErrors(getTracesIter)
			require.NoError(t, err)
			assert.Equal(t, test.expected, gotTraces)
			assert.Equal(t, test.expectedAdjustCalls, adjustCalls)
		})
	}
}

func TestArchiveTrace(t *testing.T) {
	paramsTraceIDs := []tracestore.GetTraceParams{{TraceID: testTraceID}}
	tests := []struct {
		name          string
		options       []testOption
		setupMocks    func(tqs *testQueryService)
		expectedError error
	}{
		{
			name:          "no options",
			options:       nil,
			setupMocks:    func(*testQueryService) {},
			expectedError: errNoArchiveSpanStorage,
		},
		{
			name:    "get trace error",
			options: []testOption{withArchiveTraceWriter()},
			setupMocks: func(tqs *testQueryService) {
				responseIter := iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
					yield([]ptrace.Traces{}, assert.AnError)
				})
				tqs.traceReader.On("GetTraces", mock.Anything, paramsTraceIDs).
					Return(responseIter).Once()
			},
			expectedError: assert.AnError,
		},
		{
			name:    "archive writer error",
			options: []testOption{withArchiveTraceWriter()},
			setupMocks: func(tqs *testQueryService) {
				responseIter := iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
					yield([]ptrace.Traces{makeTestTrace()}, nil)
				})
				tqs.traceReader.On("GetTraces", mock.Anything, paramsTraceIDs).
					Return(responseIter).Once()
				tqs.archiveTraceWriter.On("WriteTraces", mock.Anything, mock.AnythingOfType("ptrace.Traces")).
					Return(assert.AnError).Once()
			},
			expectedError: assert.AnError,
		},
		{
			name:    "success",
			options: []testOption{withArchiveTraceWriter()},
			setupMocks: func(tqs *testQueryService) {
				responseIter := iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
					yield([]ptrace.Traces{makeTestTrace()}, nil)
				})
				tqs.traceReader.On("GetTraces", mock.Anything, paramsTraceIDs).
					Return(responseIter).Once()
				tqs.archiveTraceWriter.On("WriteTraces", mock.Anything, mock.AnythingOfType("ptrace.Traces")).
					Return(nil).Once()
			},
			expectedError: nil,
		},
		{
			name:    "trace not found",
			options: []testOption{withArchiveTraceWriter()},
			setupMocks: func(tqs *testQueryService) {
				responseIter := iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
					yield([]ptrace.Traces{}, nil)
				})
				tqs.traceReader.On("GetTraces", mock.Anything, paramsTraceIDs).
					Return(responseIter).Once()
			},
			expectedError: spanstore.ErrTraceNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tqs := initializeTestService(test.options...)
			test.setupMocks(tqs)

			query := tracestore.GetTraceParams{
				TraceID: testTraceID,
			}

			err := tqs.queryService.ArchiveTrace(context.Background(), query)
			if test.expectedError != nil {
				require.ErrorIs(t, err, test.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGetDependencies(t *testing.T) {
	tqs := initializeTestService()
	expected := []model.DependencyLink{
		{Parent: "killer", Child: "queen", CallCount: 12},
	}
	endTs := time.Unix(0, 1476374248550*millisToNanosMultiplier)
	tqs.depsReader.On("GetDependencies", mock.Anything, depstore.QueryParameters{
		StartTime: endTs.Add(-defaultDependencyLookbackDuration),
		EndTime:   endTs,
	}).Return(expected, nil).Once()

	actualDependencies, err := tqs.queryService.GetDependencies(context.Background(), endTs, defaultDependencyLookbackDuration)
	require.NoError(t, err)
	assert.Equal(t, expected, actualDependencies)
}

func TestGetCapabilities(t *testing.T) {
	tests := []struct {
		name     string
		options  []testOption
		expected StorageCapabilities
	}{
		{
			name: "without archive storage",
			expected: StorageCapabilities{
				ArchiveStorage: false,
			},
		},
		{
			name:    "with archive storage",
			options: []testOption{withArchiveTraceReader(), withArchiveTraceWriter()},
			expected: StorageCapabilities{
				ArchiveStorage: true,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tqs := initializeTestService(test.options...)
			assert.Equal(t, test.expected, tqs.queryService.GetCapabilities())
		})
	}
}

func TestQueryServiceGetServicesReturnsEmptySlice(t *testing.T) {
	reader := new(tracestoremocks.Reader)
	reader.
		On("GetServices", mock.Anything).
		Return(nil, nil).Once()

	qs := NewQueryService(reader, nil, QueryServiceOptions{})

	services, err := qs.GetServices(context.Background())

	require.NoError(t, err)
	require.NotNil(t, services)
	require.Empty(t, services)
}

func TestGetTracesMaxTraceSize(t *testing.T) {
	tqs := initializeTestService()
	tqs.queryService.options.MaxTraceSize = 3

	params := GetTraceParams{
		TraceIDs: []tracestore.GetTraceParams{{TraceID: testTraceID}},
	}

	tqs.traceReader.On("GetTraces", mock.Anything, params.TraceIDs).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			trace := ptrace.NewTraces()
			resources := trace.ResourceSpans().AppendEmpty()
			scopes := resources.ScopeSpans().AppendEmpty()

			for i := 1; i <= 5; i++ {
				span := scopes.Spans().AppendEmpty()
				span.SetTraceID(testTraceID)
				span.SetSpanID(pcommon.SpanID([8]byte{byte(i)}))
				span.SetName(fmt.Sprintf("span-%d", i))
			}
			yield([]ptrace.Traces{trace}, nil)
		})).Once()

	getTracesIter := tqs.queryService.GetTraces(context.Background(), params)

	// Test the default aggregation route first (RawTraces = false)
	gotTracesAggTemp, errAggTemp := jiter.FlattenWithErrors(getTracesIter)
	require.NoError(t, errAggTemp)
	require.Len(t, gotTracesAggTemp, 1)

	// Now mock again for the RawTraces=true simulation
	tqs.traceReader.ExpectedCalls = nil
	tqs.traceReader.On("GetTraces", mock.Anything, params.TraceIDs).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			trace := ptrace.NewTraces()
			resources := trace.ResourceSpans().AppendEmpty()
			scopes := resources.ScopeSpans().AppendEmpty()

			for i := 1; i <= 5; i++ {
				span := scopes.Spans().AppendEmpty()
				span.SetTraceID(testTraceID)
				span.SetSpanID(pcommon.SpanID([8]byte{byte(i)}))
				span.SetName(fmt.Sprintf("span-%d", i))
			}
			yield([]ptrace.Traces{trace}, nil)
		})).Once()

	// Set RawTraces explicitly to false to aggregate
	getTracesIter = tqs.queryService.GetTraces(context.Background(), GetTraceParams{
		TraceIDs:  []tracestore.GetTraceParams{{TraceID: testTraceID}},
		RawTraces: true,
	})

	// Test the RawTraces=true route (where traces aren't aggregated by receiver, just wrapped)
	gotTracesRaw, errRaw := jiter.FlattenWithErrors(getTracesIter)
	require.NoError(t, errRaw)
	require.Len(t, gotTracesRaw, 1)

	gotSpansRaw := gotTracesRaw[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans()
	require.Equal(t, 5, gotSpansRaw.Len(), "With RawTraces=true and a single chunk returned by mock, limitTraceSize doesn't slice the inner chunk, it drops SUBSEQUENT chunks.")

	// In the single chunk, jptrace.AddWarnings should be applied to the first span
	warnings, ok := gotSpansRaw.At(0).Attributes().Get("@jaeger@warnings")
	require.True(t, ok)
	require.Equal(t, "trace size exceeded maximum allowed size", warnings.Slice().At(0).Str())

	// Now mock again for the multiple chunks simulation
	tqs.traceReader.ExpectedCalls = nil
	tqs.traceReader.On("GetTraces", mock.Anything, params.TraceIDs).
		Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
			// Chunk 1: 2 spans
			trace1 := ptrace.NewTraces()
			resources1 := trace1.ResourceSpans().AppendEmpty()
			scopes1 := resources1.ScopeSpans().AppendEmpty()
			for i := 1; i <= 2; i++ {
				span := scopes1.Spans().AppendEmpty()
				span.SetTraceID(testTraceID)
				span.SetSpanID(pcommon.SpanID([8]byte{byte(i)}))
			}
			if !yield([]ptrace.Traces{trace1}, nil) {
				return
			}

			// Chunk 2: 2 spans (exceeds limit 3!)
			trace2 := ptrace.NewTraces()
			resources2 := trace2.ResourceSpans().AppendEmpty()
			scopes2 := resources2.ScopeSpans().AppendEmpty()
			for i := 3; i <= 4; i++ {
				span := scopes2.Spans().AppendEmpty()
				span.SetTraceID(testTraceID)
				span.SetSpanID(pcommon.SpanID([8]byte{byte(i)}))
			}
			if !yield([]ptrace.Traces{trace2}, nil) {
				return
			}

			// Chunk 3: 1 span (should be totally discarded)
			trace3 := ptrace.NewTraces()
			resources3 := trace3.ResourceSpans().AppendEmpty()
			scopes3 := resources3.ScopeSpans().AppendEmpty()
			span := scopes3.Spans().AppendEmpty()
			span.SetTraceID(testTraceID)
			span.SetSpanID(pcommon.SpanID([8]byte{5}))
			yield([]ptrace.Traces{trace3}, nil)

		})).Once()

	getTracesIterAgg := tqs.queryService.GetTraces(context.Background(), GetTraceParams{
		TraceIDs:  []tracestore.GetTraceParams{{TraceID: testTraceID}},
		RawTraces: false,
	})

	gotTracesAgg, errAgg := jiter.FlattenWithErrors(getTracesIterAgg)
	require.NoError(t, errAgg)
	require.Len(t, gotTracesAgg, 1, "Aggregated into a single trace")

	require.Equal(t, 4, gotTracesAgg[0].SpanCount(), "Should only include chunk 1 and chunk 2. Chunk 3 is dropped because limit was exceeded in chunk 2.")

	// Verify warning on the first span of chunk 2 where limit was triggered
	gotSpansAggChunk2 := gotTracesAgg[0].ResourceSpans().At(1).ScopeSpans().At(0).Spans()
	warningsAgg, ok := gotSpansAggChunk2.At(0).Attributes().Get("@jaeger@warnings")
	require.True(t, ok)
	require.Equal(t, "trace size exceeded maximum allowed size", warningsAgg.Slice().At(0).Str())
}
