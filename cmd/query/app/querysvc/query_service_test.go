// Copyright (c) 2019 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package querysvc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/model/adjuster"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	depsmocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

const millisToNanosMultiplier = int64(time.Millisecond / time.Nanosecond)

var (
	errAdjustment = errors.New("adjustment error")

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

func withAdjuster() testOption {
	return func(tqs *testQueryService, options *QueryServiceOptions) {
		options.Adjuster = adjuster.Func(func(trace *model.Trace) (*model.Trace, error) {
			return trace, errAdjustment
		})
	}
}

func initializeTestService(optionAppliers ...testOption) *testQueryService {
	readStorage := &spanstoremocks.Reader{}
	dependencyStorage := &depsmocks.Reader{}

	options := QueryServiceOptions{}

	tqs := testQueryService{
		spanReader: readStorage,
		depsReader: dependencyStorage,
	}

	for _, optApplier := range optionAppliers {
		optApplier(&tqs, &options)
	}

	tqs.queryService = NewQueryService(readStorage, dependencyStorage, options)
	return &tqs
}

// Test QueryService.GetTrace()
func TestGetTraceSuccess(t *testing.T) {
	tqs := initializeTestService()
	tqs.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(mockTrace, nil).Once()

	type contextKey string
	ctx := context.Background()
	res, err := tqs.queryService.GetTrace(context.WithValue(ctx, contextKey("foo"), "bar"), mockTraceID)
	assert.NoError(t, err)
	assert.Equal(t, res, mockTrace)
}

// Test QueryService.GetTrace() without ArchiveSpanReader
func TestGetTraceNotFound(t *testing.T) {
	tqs := initializeTestService()
	tqs.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(nil, spanstore.ErrTraceNotFound).Once()

	type contextKey string
	ctx := context.Background()
	_, err := tqs.queryService.GetTrace(context.WithValue(ctx, contextKey("foo"), "bar"), mockTraceID)
	assert.Equal(t, err, spanstore.ErrTraceNotFound)
}

// Test QueryService.GetTrace() with ArchiveSpanReader
func TestGetTraceFromArchiveStorage(t *testing.T) {
	tqs := initializeTestService(withArchiveSpanReader())
	tqs.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(nil, spanstore.ErrTraceNotFound).Once()
	tqs.archiveSpanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(mockTrace, nil).Once()

	type contextKey string
	ctx := context.Background()
	res, err := tqs.queryService.GetTrace(context.WithValue(ctx, contextKey("foo"), "bar"), mockTraceID)
	assert.NoError(t, err)
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
	assert.NoError(t, err)
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
	assert.NoError(t, err)
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
	params := &spanstore.TraceQueryParameters{
		ServiceName:   "service",
		OperationName: "operation",
		StartTimeMax:  time.Now(),
		DurationMin:   duration,
		NumTraces:     200,
	}
	traces, err := tqs.queryService.FindTraces(context.WithValue(ctx, contextKey("foo"), "bar"), params)
	assert.NoError(t, err)
	assert.Len(t, traces, 1)
}

// Test QueryService.ArchiveTrace() with no ArchiveSpanWriter.
func TestArchiveTraceNoOptions(t *testing.T) {
	tqs := initializeTestService()

	type contextKey string
	ctx := context.Background()
	err := tqs.queryService.ArchiveTrace(context.WithValue(ctx, contextKey("foo"), "bar"), mockTraceID)
	assert.Equal(t, errNoArchiveSpanStorage, err)
}

// Test QueryService.ArchiveTrace() with ArchiveSpanWriter but invalid traceID.
func TestArchiveTraceWithInvalidTraceID(t *testing.T) {
	tqs := initializeTestService(withArchiveSpanReader(), withArchiveSpanWriter())
	tqs.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(nil, spanstore.ErrTraceNotFound).Once()
	tqs.archiveSpanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(nil, spanstore.ErrTraceNotFound).Once()

	type contextKey string
	ctx := context.Background()
	err := tqs.queryService.ArchiveTrace(context.WithValue(ctx, contextKey("foo"), "bar"), mockTraceID)
	assert.Equal(t, spanstore.ErrTraceNotFound, err)
}

// Test QueryService.ArchiveTrace(), save error with ArchiveSpanWriter.
func TestArchiveTraceWithArchiveWriterError(t *testing.T) {
	tqs := initializeTestService(withArchiveSpanWriter())
	tqs.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(mockTrace, nil).Once()
	tqs.archiveSpanWriter.On("WriteSpan", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*model.Span")).
		Return(errors.New("cannot save")).Times(2)

	type contextKey string
	ctx := context.Background()
	multiErr := tqs.queryService.ArchiveTrace(context.WithValue(ctx, contextKey("foo"), "bar"), mockTraceID)
	assert.Len(t, multiErr, 2)
	// There are two spans in the mockTrace, ArchiveTrace should return a wrapped error.
	assert.EqualError(t, multiErr, "[cannot save, cannot save]")
}

// Test QueryService.ArchiveTrace() with correctly configured ArchiveSpanWriter.
func TestArchiveTraceSuccess(t *testing.T) {
	tqs := initializeTestService(withArchiveSpanWriter())
	tqs.spanReader.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(mockTrace, nil).Once()
	tqs.archiveSpanWriter.On("WriteSpan", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*model.Span")).
		Return(nil).Times(2)

	type contextKey string
	ctx := context.Background()
	err := tqs.queryService.ArchiveTrace(context.WithValue(ctx, contextKey("foo"), "bar"), mockTraceID)
	assert.NoError(t, err)
}

// Test QueryService.Adjust()
func TestTraceAdjustmentFailure(t *testing.T) {
	tqs := initializeTestService(withAdjuster())

	_, err := tqs.queryService.Adjust(mockTrace)
	assert.Error(t, err)
	assert.EqualValues(t, errAdjustment.Error(), err.Error())
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
	tqs.depsReader.On("GetDependencies", endTs, defaultDependencyLookbackDuration).Return(expectedDependencies, nil).Times(1)

	actualDependencies, err := tqs.queryService.GetDependencies(context.Background(), time.Unix(0, 1476374248550*millisToNanosMultiplier), defaultDependencyLookbackDuration)
	assert.NoError(t, err)
	assert.Equal(t, expectedDependencies, actualDependencies)
}

type fakeStorageFactory1 struct {
}

type fakeStorageFactory2 struct {
	fakeStorageFactory1
	r    spanstore.Reader
	w    spanstore.Writer
	rErr error
	wErr error
}

func (*fakeStorageFactory1) Initialize(metricsFactory metrics.Factory, logger *zap.Logger) error {
	return nil
}
func (*fakeStorageFactory1) CreateSpanReader() (spanstore.Reader, error)             { return nil, nil }
func (*fakeStorageFactory1) CreateSpanWriter() (spanstore.Writer, error)             { return nil, nil }
func (*fakeStorageFactory1) CreateDependencyReader() (dependencystore.Reader, error) { return nil, nil }

func (f *fakeStorageFactory2) CreateArchiveSpanReader() (spanstore.Reader, error) { return f.r, f.rErr }
func (f *fakeStorageFactory2) CreateArchiveSpanWriter() (spanstore.Writer, error) { return f.w, f.wErr }

var _ storage.Factory = new(fakeStorageFactory1)
var _ storage.ArchiveFactory = new(fakeStorageFactory2)

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
