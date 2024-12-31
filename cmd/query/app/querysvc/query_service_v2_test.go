// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/iter"
	"github.com/jaegertracing/jaeger/storage_v2/depstore"
	depstoremocks "github.com/jaegertracing/jaeger/storage_v2/depstore/mocks"
	"github.com/jaegertracing/jaeger/storage_v2/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/storage_v2/tracestore/mocks"
)

const millisToNanosMultiplier = int64(time.Millisecond / time.Nanosecond)

var (
	defaultDependencyLookbackDuration = time.Hour * 24

	testTraceID = pcommon.TraceID([16]byte{1})
)

type testQueryServiceV2 struct {
	queryService *QueryServiceV2
	traceReader  *tracestoremocks.Reader
	depsReader   *depstoremocks.Reader

	archiveTraceReader *tracestoremocks.Reader
	archiveTraceWriter *tracestoremocks.Writer
}

type testOptionV2 func(*testQueryServiceV2, *QueryServiceOptionsV2)

func withArchiveTraceReader() testOptionV2 {
	return func(tqs *testQueryServiceV2, options *QueryServiceOptionsV2) {
		r := &tracestoremocks.Reader{}
		tqs.archiveTraceReader = r
		options.ArchiveTraceReader = r
	}
}

func withArchiveTraceWriter() testOptionV2 {
	return func(tqs *testQueryServiceV2, options *QueryServiceOptionsV2) {
		r := &tracestoremocks.Writer{}
		tqs.archiveTraceWriter = r
		options.ArchiveTraceWriter = r
	}
}

func initializeTestServiceV2(opts ...testOptionV2) *testQueryServiceV2 {
	traceReader := &tracestoremocks.Reader{}
	dependencyStorage := &depstoremocks.Reader{}

	options := QueryServiceOptionsV2{}

	tqs := testQueryServiceV2{
		traceReader: traceReader,
		depsReader:  dependencyStorage,
	}

	for _, opt := range opts {
		opt(&tqs, &options)
	}

	tqs.queryService = NewQueryServiceV2(traceReader, dependencyStorage, options)
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
	spanB.Attributes()

	return trace
}

func TestGetTracesSuccess(t *testing.T) {
	responseIter := iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
		yield([]ptrace.Traces{makeTestTrace()}, nil)
	})

	tqs := initializeTestServiceV2()
	tqs.traceReader.On("GetTraces", mock.Anything, tracestore.GetTraceParams{TraceID: testTraceID}).
		Return(responseIter, nil).Once()

	params := GetTraceParams{
		TraceIDs: []tracestore.GetTraceParams{
			{
				TraceID: testTraceID,
			},
		},
	}
	var gotTraces []ptrace.Traces
	getTracesIter := tqs.queryService.GetTraces(context.Background(), params)
	getTracesIter(func(traces []ptrace.Traces, _ error) bool {
		gotTraces = append(gotTraces, traces...)
		return true
	})

	require.Len(t, gotTraces, 1)
	gotSpans := gotTraces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans()
	require.Equal(t, 2, gotSpans.Len())
	require.Equal(t, testTraceID, gotSpans.At(0).TraceID())
	require.EqualValues(t, [8]byte{1}, gotSpans.At(0).SpanID())
	require.Equal(t, testTraceID, gotSpans.At(1).TraceID())
	require.EqualValues(t, [8]byte{2}, gotSpans.At(1).SpanID())
}

func TestGetTracesWithRawTraces(t *testing.T) {
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

			tqs := initializeTestServiceV2()
			tqs.traceReader.On("GetTraces", mock.Anything, tracestore.GetTraceParams{TraceID: testTraceID}).
				Return(responseIter, nil).Once()

			params := GetTraceParams{
				TraceIDs: []tracestore.GetTraceParams{
					{
						TraceID: testTraceID,
					},
				},
				RawTraces: test.rawTraces,
			}
			var gotTraces []ptrace.Traces
			getTracesIter := tqs.queryService.GetTraces(context.Background(), params)
			getTracesIter(func(traces []ptrace.Traces, _ error) bool {
				gotTraces = append(gotTraces, traces...)
				return true
			})
			require.Len(t, gotTraces, 1)
			gotAttributes := gotTraces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
			require.Equal(t, test.expected, gotAttributes)
		})
	}
}

func TestGetTraces_TraceInArchiveStorage(t *testing.T) {
	responseIter := iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
		yield([]ptrace.Traces{}, nil)
	})

	archiveResponseIter := iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
		yield([]ptrace.Traces{makeTestTrace()}, nil)
	})

	tqs := initializeTestServiceV2(withArchiveTraceReader())
	tqs.traceReader.On("GetTraces", mock.Anything, tracestore.GetTraceParams{TraceID: testTraceID}).
		Return(responseIter, nil).Once()
	tqs.archiveTraceReader.On("GetTraces", mock.Anything, tracestore.GetTraceParams{TraceID: testTraceID}).
		Return(archiveResponseIter, nil).Once()

	params := GetTraceParams{
		TraceIDs: []tracestore.GetTraceParams{
			{
				TraceID: testTraceID,
			},
		},
	}
	var gotTraces []ptrace.Traces
	getTracesIter := tqs.queryService.GetTraces(context.Background(), params)
	getTracesIter(func(traces []ptrace.Traces, _ error) bool {
		gotTraces = append(gotTraces, traces...)
		return true
	})

	require.Len(t, gotTraces, 1)
	gotSpans := gotTraces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans()
	require.Equal(t, 2, gotSpans.Len())
	require.Equal(t, testTraceID, gotSpans.At(0).TraceID())
	require.EqualValues(t, [8]byte{1}, gotSpans.At(0).SpanID())
	require.Equal(t, testTraceID, gotSpans.At(1).TraceID())
	require.EqualValues(t, [8]byte{2}, gotSpans.At(1).SpanID())
}

func TestGetServicesV2(t *testing.T) {
	tqs := initializeTestServiceV2()
	expected := []string{"trifle", "bling"}
	tqs.traceReader.On("GetServices", mock.Anything).Return(expected, nil).Once()

	actualServices, err := tqs.queryService.GetServices(context.Background())
	require.NoError(t, err)
	assert.Equal(t, expected, actualServices)
}

func TestGetOperationsV2(t *testing.T) {
	tqs := initializeTestServiceV2()
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

func TestFindTracesV2(t *testing.T) {
	responseIter := iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
		yield([]ptrace.Traces{makeTestTrace()}, nil)
	})

	tqs := initializeTestServiceV2()
	duration, err := time.ParseDuration("20ms")
	require.NoError(t, err)
	now := time.Now()
	tqs.traceReader.On("FindTraces", mock.Anything, tracestore.TraceQueryParams{
		ServiceName:   "service",
		OperationName: "operation",
		StartTimeMax:  now,
		DurationMin:   duration,
		NumTraces:     200,
	}).
		Return(responseIter, nil).Once()

	query := TraceQueryParams{
		TraceQueryParams: tracestore.TraceQueryParams{
			ServiceName:   "service",
			OperationName: "operation",
			StartTimeMax:  now,
			DurationMin:   duration,
			NumTraces:     200,
		},
	}
	var gotTraces []ptrace.Traces
	getTracesIter := tqs.queryService.FindTraces(context.Background(), query)
	getTracesIter(func(traces []ptrace.Traces, _ error) bool {
		gotTraces = append(gotTraces, traces...)
		return true
	})

	require.Len(t, gotTraces, 1)
	gotSpans := gotTraces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans()
	require.Equal(t, 2, gotSpans.Len())
	require.Equal(t, testTraceID, gotSpans.At(0).TraceID())
	require.EqualValues(t, [8]byte{1}, gotSpans.At(0).SpanID())
	require.Equal(t, testTraceID, gotSpans.At(1).TraceID())
	require.EqualValues(t, [8]byte{2}, gotSpans.At(1).SpanID())
}

func TestFindTracesWithRawTracesV2(t *testing.T) {
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

			tqs := initializeTestServiceV2()
			duration, err := time.ParseDuration("20ms")
			require.NoError(t, err)
			now := time.Now()
			tqs.traceReader.On("FindTraces", mock.Anything, tracestore.TraceQueryParams{
				ServiceName:   "service",
				OperationName: "operation",
				StartTimeMax:  now,
				DurationMin:   duration,
				NumTraces:     200,
			}).
				Return(responseIter, nil).Once()

			query := TraceQueryParams{
				TraceQueryParams: tracestore.TraceQueryParams{
					ServiceName:   "service",
					OperationName: "operation",
					StartTimeMax:  now,
					DurationMin:   duration,
					NumTraces:     200,
				},
				RawTraces: test.rawTraces,
			}
			var gotTraces []ptrace.Traces
			getTracesIter := tqs.queryService.FindTraces(context.Background(), query)
			getTracesIter(func(traces []ptrace.Traces, _ error) bool {
				gotTraces = append(gotTraces, traces...)
				return true
			})
			require.Len(t, gotTraces, 1)
			gotAttributes := gotTraces[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
			require.Equal(t, test.expected, gotAttributes)
		})
	}
}

func TestArchiveTraceV2_NoOptions(t *testing.T) {
	tqs := initializeTestServiceV2()

	type contextKey string
	ctx := context.Background()
	query := tracestore.GetTraceParams{
		TraceID: testTraceID,
	}

	err := tqs.queryService.ArchiveTrace(context.WithValue(ctx, contextKey("foo"), "bar"), query)
	assert.Equal(t, errNoArchiveSpanStorage, err)
}

func TestArchiveTraceV2_ArchiveWriterError(t *testing.T) {
	tqs := initializeTestServiceV2(withArchiveTraceWriter())

	responseIter := iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
		yield([]ptrace.Traces{makeTestTrace()}, nil)
	})

	tqs.traceReader.On("GetTraces", mock.Anything, tracestore.GetTraceParams{TraceID: testTraceID}).
		Return(responseIter, nil).Once()
	tqs.archiveTraceWriter.On("WriteTraces", mock.Anything, mock.AnythingOfType("ptrace.Traces")).
		Return(assert.AnError).Once()

	query := tracestore.GetTraceParams{
		TraceID: testTraceID,
	}

	err := tqs.queryService.ArchiveTrace(context.Background(), query)
	require.ErrorIs(t, err, assert.AnError)
}

func TestArchiveTraceV2_Success(t *testing.T) {
	tqs := initializeTestServiceV2(withArchiveTraceWriter())

	responseIter := iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
		yield([]ptrace.Traces{makeTestTrace()}, nil)
	})

	tqs.traceReader.On("GetTraces", mock.Anything, tracestore.GetTraceParams{TraceID: testTraceID}).
		Return(responseIter, nil).Once()
	tqs.archiveTraceWriter.On("WriteTraces", mock.Anything, mock.AnythingOfType("ptrace.Traces")).
		Return(nil).Once()

	query := tracestore.GetTraceParams{
		TraceID: testTraceID,
	}

	err := tqs.queryService.ArchiveTrace(context.Background(), query)
	require.NoError(t, err)
}

func TestGetDependenciesV2(t *testing.T) {
	tqs := initializeTestServiceV2()
	expected := []model.DependencyLink{
		{
			Parent:    "killer",
			Child:     "queen",
			CallCount: 12,
		},
	}
	endTs := time.Unix(0, 1476374248550*millisToNanosMultiplier)
	tqs.depsReader.On(
		"GetDependencies",
		mock.Anything, // context.Context
		depstore.QueryParameters{
			StartTime: endTs.Add(-defaultDependencyLookbackDuration),
			EndTime:   endTs,
		}).Return(expected, nil).Times(1)

	actualDependencies, err := tqs.queryService.GetDependencies(
		context.Background(), endTs,
		defaultDependencyLookbackDuration)
	require.NoError(t, err)
	assert.Equal(t, expected, actualDependencies)
}

func TestGetCapabilitiesV2(t *testing.T) {
	tqs := initializeTestServiceV2()
	expected := StorageCapabilities{
		ArchiveStorage: false,
	}
	assert.Equal(t, expected, tqs.queryService.GetCapabilities())
}

func TestGetCapabilitiesWithSupportsArchiveV2(t *testing.T) {
	tqs := initializeTestServiceV2(withArchiveTraceReader(), withArchiveTraceWriter())

	expected := StorageCapabilities{
		ArchiveStorage: true,
	}
	assert.Equal(t, expected, tqs.queryService.GetCapabilities())
}
