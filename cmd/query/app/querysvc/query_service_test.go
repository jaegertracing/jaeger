// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
	"github.com/jaegertracing/jaeger/storage_v2/depstore"
	depsmocks "github.com/jaegertracing/jaeger/storage_v2/depstore/mocks"
	tracestoremocks "github.com/jaegertracing/jaeger/storage_v2/tracestore/mocks"
	"github.com/jaegertracing/jaeger/storage_v2/v1adapter"
)

const millisToNanosMultiplier = int64(time.Millisecond / time.Nanosecond)

var (
	defaultDependencyLookbackDuration = time.Hour * 24

	mockTraceID = model.NewTraceID(0, 123456)
	mockTrace   = &model.Trace{
		Spans: []*model.Span{
			{
				TraceID: mockTraceID,
				SpanID:  model.NewSpanID(1),
				Process: &model.Process{},
			},
			{
				TraceID: mockTraceID,
				SpanID:  model.NewSpanID(2),
				Process: &model.Process{},
			},
		},
		Warnings: []string{},
	}
)

type testQueryService struct {
	queryService *QueryService
	spanReader   *spanstoremocks.Reader
	depsReader   *depsmocks.Reader

	archiveSpanReader *spanstoremocks.Reader
	archiveSpanWriter *spanstoremocks.Writer
}

type testOption func(*testQueryService, *QueryServiceOptions)

func withArchiveSpanReader() testOption {
	return func(tqs *testQueryService, options *QueryServiceOptions) {
		r := &spanstoremocks.Reader{}
		tqs.archiveSpanReader = r
		options.ArchiveSpanReader = r
	}
}

func withArchiveSpanWriter() testOption {
	return func(tqs *testQueryService, options *QueryServiceOptions) {
		r := &spanstoremocks.Writer{}
		tqs.archiveSpanWriter = r
		options.ArchiveSpanWriter = r
	}
}

func initializeTestService(optionAppliers ...testOption) *testQueryService {
	readStorage := &spanstoremocks.Reader{}
	traceReader := v1adapter.NewTraceReader(readStorage)
	dependencyStorage := &depsmocks.Reader{}

	options := QueryServiceOptions{}

	tqs := testQueryService{
		spanReader: readStorage,
		depsReader: dependencyStorage,
	}

	for _, optApplier := range optionAppliers {
		optApplier(&tqs, &options)
	}

	tqs.queryService = NewQueryService(traceReader, dependencyStorage, options)
	return &tqs
}

// Test QueryService.GetTrace()
func TestGetTraceSuccess(t *testing.T) {
	tqs := initializeTestService()
	tqs.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
		Return(mockTrace, nil).Once()

	type contextKey string
	ctx := context.Background()
	query := GetTraceParameters{
		GetTraceParameters: spanstore.GetTraceParameters{
			TraceID: mockTraceID,
		},
	}
	res, err := tqs.queryService.GetTrace(context.WithValue(ctx, contextKey("foo"), "bar"), query)
	require.NoError(t, err)
	assert.Equal(t, res, mockTrace)
}

func TestGetTraceWithRawTraces(t *testing.T) {
	traceID := model.NewTraceID(0, 1)
	tests := []struct {
		rawTraces bool
		tags      model.KeyValues
		expected  model.KeyValues
	}{
		{
			// tags should not get sorted by SortTagsAndLogFields adjuster
			rawTraces: true,
			tags: model.KeyValues{
				model.String("z", "key"),
				model.String("a", "key"),
			},
			expected: model.KeyValues{
				model.String("z", "key"),
				model.String("a", "key"),
			},
		},
		{
			// tags should get sorted by SortTagsAndLogFields adjuster
			rawTraces: false,
			tags: model.KeyValues{
				model.String("z", "key"),
				model.String("a", "key"),
			},
			expected: model.KeyValues{
				model.String("a", "key"),
				model.String("z", "key"),
			},
		},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("rawTraces=%v", test.rawTraces), func(t *testing.T) {
			trace := &model.Trace{
				Spans: []*model.Span{
					{
						TraceID: traceID,
						SpanID:  model.NewSpanID(1),
						Process: &model.Process{},
						Tags:    test.tags,
					},
				},
			}
			tqs := initializeTestService()
			tqs.spanReader.On("GetTrace", mock.Anything, mock.AnythingOfType("spanstore.GetTraceParameters")).
				Return(trace, nil).Once()
			query := GetTraceParameters{
				GetTraceParameters: spanstore.GetTraceParameters{
					TraceID: mockTraceID,
				},
				RawTraces: test.rawTraces,
			}
			gotTrace, err := tqs.queryService.GetTrace(context.Background(), query)
			require.NoError(t, err)
			spans := gotTrace.Spans
			require.Len(t, spans, 1)
			require.EqualValues(t, test.expected, spans[0].Tags)
		})
	}
}

// Test QueryService.GetTrace() without ArchiveSpanReader
func TestGetTraceNotFound(t *testing.T) {
	tqs := initializeTestService()
	tqs.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
		Return(nil, spanstore.ErrTraceNotFound).Once()

	type contextKey string
	ctx := context.Background()
	query := GetTraceParameters{
		GetTraceParameters: spanstore.GetTraceParameters{
			TraceID: mockTraceID,
		},
	}
	_, err := tqs.queryService.GetTrace(context.WithValue(ctx, contextKey("foo"), "bar"), query)
	assert.Equal(t, err, spanstore.ErrTraceNotFound)
}

// Test QueryService.GetTrace() with ArchiveSpanReader
func TestGetTraceFromArchiveStorage(t *testing.T) {
	tqs := initializeTestService(withArchiveSpanReader())
	tqs.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
		Return(nil, spanstore.ErrTraceNotFound).Once()
	tqs.archiveSpanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
		Return(mockTrace, nil).Once()

	type contextKey string
	ctx := context.Background()
	query := GetTraceParameters{
		GetTraceParameters: spanstore.GetTraceParameters{
			TraceID: mockTraceID,
		},
	}
	res, err := tqs.queryService.GetTrace(context.WithValue(ctx, contextKey("foo"), "bar"), query)
	require.NoError(t, err)
	assert.Equal(t, res, mockTrace)
}

// Test QueryService.GetServices() for success.
func TestGetServices(t *testing.T) {
	tqs := initializeTestService()
	expectedServices := []string{"trifle", "bling"}
	tqs.spanReader.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(expectedServices, nil).Once()

	type contextKey string
	ctx := context.Background()
	actualServices, err := tqs.queryService.GetServices(context.WithValue(ctx, contextKey("foo"), "bar"))
	require.NoError(t, err)
	assert.Equal(t, expectedServices, actualServices)
}

// Test QueryService.GetOperations() for success.
func TestGetOperations(t *testing.T) {
	tqs := initializeTestService()
	expectedOperations := []spanstore.Operation{{Name: "", SpanKind: ""}, {Name: "get", SpanKind: ""}}
	operationQuery := spanstore.OperationQueryParameters{ServiceName: "abc/trifle"}
	tqs.spanReader.On(
		"GetOperations",
		mock.AnythingOfType("*context.valueCtx"),
		operationQuery,
	).Return(expectedOperations, nil).Once()

	type contextKey string
	ctx := context.Background()
	actualOperations, err := tqs.queryService.GetOperations(context.WithValue(ctx, contextKey("foo"), "bar"), operationQuery)
	require.NoError(t, err)
	assert.Equal(t, expectedOperations, actualOperations)
}

// Test QueryService.FindTraces() for success.
func TestFindTraces(t *testing.T) {
	tqs := initializeTestService()
	tqs.spanReader.On("FindTraces", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*spanstore.TraceQueryParameters")).
		Return([]*model.Trace{mockTrace}, nil).Once()

	type contextKey string
	ctx := context.Background()
	duration, _ := time.ParseDuration("20ms")
	params := &TraceQueryParameters{
		TraceQueryParameters: spanstore.TraceQueryParameters{
			ServiceName:   "service",
			OperationName: "operation",
			StartTimeMax:  time.Now(),
			DurationMin:   duration,
			NumTraces:     200,
		},
	}
	traces, err := tqs.queryService.FindTraces(context.WithValue(ctx, contextKey("foo"), "bar"), params)
	require.NoError(t, err)
	assert.Len(t, traces, 1)
}

func TestFindTracesWithRawTraces(t *testing.T) {
	traceID := model.NewTraceID(0, 1)
	tests := []struct {
		rawTraces bool
		tags      model.KeyValues
		expected  model.KeyValues
	}{
		{
			// tags should not get sorted by SortTagsAndLogFields adjuster
			rawTraces: true,
			tags: model.KeyValues{
				model.String("z", "key"),
				model.String("a", "key"),
			},
			expected: model.KeyValues{
				model.String("z", "key"),
				model.String("a", "key"),
			},
		},
		{
			// tags should get sorted by SortTagsAndLogFields adjuster
			rawTraces: false,
			tags: model.KeyValues{
				model.String("z", "key"),
				model.String("a", "key"),
			},
			expected: model.KeyValues{
				model.String("a", "key"),
				model.String("z", "key"),
			},
		},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("rawTraces=%v", test.rawTraces), func(t *testing.T) {
			trace := &model.Trace{
				Spans: []*model.Span{
					{
						TraceID: traceID,
						SpanID:  model.NewSpanID(1),
						Process: &model.Process{},
						Tags:    test.tags,
					},
				},
			}
			tqs := initializeTestService()
			tqs.spanReader.On("FindTraces", mock.Anything, mock.AnythingOfType("*spanstore.TraceQueryParameters")).
				Return([]*model.Trace{trace}, nil).Once()
			params := &TraceQueryParameters{
				RawTraces: test.rawTraces,
			}
			traces, err := tqs.queryService.FindTraces(context.Background(), params)
			require.NoError(t, err)
			require.Len(t, traces, 1)
			spans := traces[0].Spans
			require.Len(t, spans, 1)
			require.EqualValues(t, test.expected, spans[0].Tags)
		})
	}
}

func TestFindTracesError(t *testing.T) {
	tqs := initializeTestService()
	tqs.spanReader.On("FindTraces", mock.Anything, mock.AnythingOfType("*spanstore.TraceQueryParameters")).
		Return(nil, assert.AnError).Once()
	traces, err := tqs.queryService.FindTraces(context.Background(), &TraceQueryParameters{})
	require.ErrorIs(t, err, assert.AnError)
	require.Nil(t, traces)
}

// Test QueryService.ArchiveTrace() with no ArchiveSpanWriter.
func TestArchiveTraceNoOptions(t *testing.T) {
	tqs := initializeTestService()

	type contextKey string
	ctx := context.Background()
	query := spanstore.GetTraceParameters{
		TraceID: mockTraceID,
	}

	err := tqs.queryService.ArchiveTrace(context.WithValue(ctx, contextKey("foo"), "bar"), query)
	assert.Equal(t, errNoArchiveSpanStorage, err)
}

// Test QueryService.ArchiveTrace() with ArchiveSpanWriter but invalid traceID.
func TestArchiveTraceWithInvalidTraceID(t *testing.T) {
	tqs := initializeTestService(withArchiveSpanReader(), withArchiveSpanWriter())
	tqs.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
		Return(nil, spanstore.ErrTraceNotFound).Once()
	tqs.archiveSpanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
		Return(nil, spanstore.ErrTraceNotFound).Once()

	type contextKey string
	ctx := context.Background()
	query := spanstore.GetTraceParameters{
		TraceID: mockTraceID,
	}
	err := tqs.queryService.ArchiveTrace(context.WithValue(ctx, contextKey("foo"), "bar"), query)
	assert.Equal(t, spanstore.ErrTraceNotFound, err)
}

// Test QueryService.ArchiveTrace(), save error with ArchiveSpanWriter.
func TestArchiveTraceWithArchiveWriterError(t *testing.T) {
	tqs := initializeTestService(withArchiveSpanWriter())
	tqs.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
		Return(mockTrace, nil).Once()
	tqs.archiveSpanWriter.On("WriteSpan", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*model.Span")).
		Return(errors.New("cannot save")).Times(2)

	type contextKey string
	ctx := context.Background()
	query := spanstore.GetTraceParameters{
		TraceID: mockTraceID,
	}

	joinErr := tqs.queryService.ArchiveTrace(context.WithValue(ctx, contextKey("foo"), "bar"), query)
	// There are two spans in the mockTrace, ArchiveTrace should return a wrapped error.
	require.EqualError(t, joinErr, "cannot save\ncannot save")
}

// Test QueryService.ArchiveTrace() with correctly configured ArchiveSpanWriter.
func TestArchiveTraceSuccess(t *testing.T) {
	tqs := initializeTestService(withArchiveSpanWriter())
	tqs.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("spanstore.GetTraceParameters")).
		Return(mockTrace, nil).Once()
	tqs.archiveSpanWriter.On("WriteSpan", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*model.Span")).
		Return(nil).Times(2)

	type contextKey string
	ctx := context.Background()
	query := spanstore.GetTraceParameters{
		TraceID: mockTraceID,
	}

	err := tqs.queryService.ArchiveTrace(context.WithValue(ctx, contextKey("foo"), "bar"), query)
	require.NoError(t, err)
}

// Test QueryService.GetDependencies()
func TestGetDependencies(t *testing.T) {
	tqs := initializeTestService()
	expectedDependencies := []model.DependencyLink{
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
		}).Return(expectedDependencies, nil).Times(1)

	actualDependencies, err := tqs.queryService.GetDependencies(context.Background(), time.Unix(0, 1476374248550*millisToNanosMultiplier), defaultDependencyLookbackDuration)
	require.NoError(t, err)
	assert.Equal(t, expectedDependencies, actualDependencies)
}

// Test QueryService.GetCapacities()
func TestGetCapabilities(t *testing.T) {
	tqs := initializeTestService()
	expectedStorageCapabilities := StorageCapabilities{
		ArchiveStorage: false,
	}
	assert.Equal(t, expectedStorageCapabilities, tqs.queryService.GetCapabilities())
}

func TestGetCapabilitiesWithSupportsArchive(t *testing.T) {
	tqs := initializeTestService(withArchiveSpanReader(), withArchiveSpanWriter())

	expectedStorageCapabilities := StorageCapabilities{
		ArchiveStorage: true,
	}
	assert.Equal(t, expectedStorageCapabilities, tqs.queryService.GetCapabilities())
}

type fakeStorageFactory1 struct{}

type fakeStorageFactory2 struct {
	fakeStorageFactory1
	r    spanstore.Reader
	w    spanstore.Writer
	rErr error
	wErr error
}

func (*fakeStorageFactory1) Initialize(metrics.Factory, *zap.Logger) error {
	return nil
}
func (*fakeStorageFactory1) CreateSpanReader() (spanstore.Reader, error)             { return nil, nil }
func (*fakeStorageFactory1) CreateSpanWriter() (spanstore.Writer, error)             { return nil, nil }
func (*fakeStorageFactory1) CreateDependencyReader() (dependencystore.Reader, error) { return nil, nil }

func (f *fakeStorageFactory2) CreateArchiveSpanReader() (spanstore.Reader, error) { return f.r, f.rErr }
func (f *fakeStorageFactory2) CreateArchiveSpanWriter() (spanstore.Writer, error) { return f.w, f.wErr }

var (
	_ storage.Factory        = new(fakeStorageFactory1)
	_ storage.ArchiveFactory = new(fakeStorageFactory2)
)

func TestInitArchiveStorageErrors(t *testing.T) {
	opts := &QueryServiceOptions{}
	logger := zap.NewNop()

	assert.False(t, opts.InitArchiveStorage(new(fakeStorageFactory1), logger))
	assert.False(t, opts.InitArchiveStorage(
		&fakeStorageFactory2{rErr: storage.ErrArchiveStorageNotConfigured},
		logger,
	))
	assert.False(t, opts.InitArchiveStorage(
		&fakeStorageFactory2{rErr: errors.New("error")},
		logger,
	))
	assert.False(t, opts.InitArchiveStorage(
		&fakeStorageFactory2{wErr: storage.ErrArchiveStorageNotConfigured},
		logger,
	))
	assert.False(t, opts.InitArchiveStorage(
		&fakeStorageFactory2{wErr: errors.New("error")},
		logger,
	))
}

func TestInitArchiveStorage(t *testing.T) {
	opts := &QueryServiceOptions{}
	logger := zap.NewNop()
	reader := &spanstoremocks.Reader{}
	writer := &spanstoremocks.Writer{}
	assert.True(t, opts.InitArchiveStorage(
		&fakeStorageFactory2{r: reader, w: writer},
		logger,
	))
	assert.Equal(t, reader, opts.ArchiveSpanReader)
	assert.Equal(t, writer, opts.ArchiveSpanWriter)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

func TestNewQueryService_PanicsForNonV1AdapterReader(t *testing.T) {
	reader := &tracestoremocks.Reader{}
	dependencyReader := &depsmocks.Reader{}
	options := QueryServiceOptions{}
	require.PanicsWithError(t, v1adapter.ErrV1ReaderNotAvailable.Error(), func() { NewQueryService(reader, dependencyReader, options) })
}
