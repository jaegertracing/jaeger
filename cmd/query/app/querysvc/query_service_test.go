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

func initializeTestServiceWithArchiveOptions() (*QueryService, *spanstoremocks.Reader, *depsmocks.Reader, *spanstoremocks.Reader, *spanstoremocks.Writer) {
	readStorage := &spanstoremocks.Reader{}
	dependencyStorage := &depsmocks.Reader{}
	archiveReadStorage := &spanstoremocks.Reader{}
	archiveWriteStorage := &spanstoremocks.Writer{}
	options := QueryServiceOptions{
		ArchiveSpanReader: archiveReadStorage,
		ArchiveSpanWriter: archiveWriteStorage,
	}
	qs := NewQueryService(readStorage, dependencyStorage, options)
	return qs, readStorage, dependencyStorage, archiveReadStorage, archiveWriteStorage
}

func initializeTestServiceWithAdjustOption() *QueryService {
	readStorage := &spanstoremocks.Reader{}
	dependencyStorage := &depsmocks.Reader{}
	options := QueryServiceOptions{
		Adjuster: adjuster.Func(func(trace *model.Trace) (*model.Trace, error) {
			return trace, errAdjustment
		}),
	}
	qs := NewQueryService(readStorage, dependencyStorage, options)
	return qs
}

func initializeTestService() (*QueryService, *spanstoremocks.Reader, *depsmocks.Reader) {
	readStorage := &spanstoremocks.Reader{}
	dependencyStorage := &depsmocks.Reader{}
	qs := NewQueryService(readStorage, dependencyStorage, QueryServiceOptions{})
	return qs, readStorage, dependencyStorage
}

// Test QueryService.GetTrace()
func TestGetTraceSuccess(t *testing.T) {
	qs, readMock, _ := initializeTestService()
	readMock.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(mockTrace, nil).Once()

	type contextKey string
	ctx := context.Background()
	res, err := qs.GetTrace(context.WithValue(ctx, contextKey("foo"), "bar"), mockTraceID)
	assert.NoError(t, err)
	assert.Equal(t, res, mockTrace)
}

// Test QueryService.GetTrace() without ArchiveSpanReader
func TestGetTraceNotFound(t *testing.T) {
	qs, readMock, _ := initializeTestService()
	readMock.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(nil, spanstore.ErrTraceNotFound).Once()

	type contextKey string
	ctx := context.Background()
	_, err := qs.GetTrace(context.WithValue(ctx, contextKey("foo"), "bar"), mockTraceID)
	assert.Equal(t, err, spanstore.ErrTraceNotFound)
}

// Test QueryService.GetTrace() with ArchiveSpanReader
func TestGetTraceFromArchiveStorage(t *testing.T) {
	qs, readMock, _, readArchiveMock, _ := initializeTestServiceWithArchiveOptions()
	readMock.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(nil, spanstore.ErrTraceNotFound).Once()
	readArchiveMock.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(mockTrace, nil).Once()

	type contextKey string
	ctx := context.Background()
	res, err := qs.GetTrace(context.WithValue(ctx, contextKey("foo"), "bar"), mockTraceID)
	assert.NoError(t, err)
	assert.Equal(t, res, mockTrace)
}

// Test QueryService.GetServices() for success.
func TestGetServices(t *testing.T) {
	qs, readMock, _ := initializeTestService()
	expectedServices := []string{"trifle", "bling"}
	readMock.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(expectedServices, nil).Once()

	type contextKey string
	ctx := context.Background()
	actualServices, err := qs.GetServices(context.WithValue(ctx, contextKey("foo"), "bar"))
	assert.NoError(t, err)
	assert.Equal(t, expectedServices, actualServices)
}

// Test QueryService.GetOperations() for success.
func TestGetOperations(t *testing.T) {
	qs, readMock, _ := initializeTestService()
	expectedOperations := []spanstore.Operation{{Name: "", SpanKind: ""}, {Name: "get", SpanKind: ""}}
	operationQuery := spanstore.OperationQueryParameters{ServiceName: "abc/trifle"}
	readMock.On(
		"GetOperations",
		mock.AnythingOfType("*context.valueCtx"),
		operationQuery,
	).Return(expectedOperations, nil).Once()

	type contextKey string
	ctx := context.Background()
	actualOperations, err := qs.GetOperations(context.WithValue(ctx, contextKey("foo"), "bar"), operationQuery)
	assert.NoError(t, err)
	assert.Equal(t, expectedOperations, actualOperations)
}

// Test QueryService.FindTraces() for success.
func TestFindTraces(t *testing.T) {
	qs, readMock, _ := initializeTestService()
	readMock.On("FindTraces", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*spanstore.TraceQueryParameters")).
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
	traces, err := qs.FindTraces(context.WithValue(ctx, contextKey("foo"), "bar"), params)
	assert.NoError(t, err)
	assert.Len(t, traces, 1)
}

// Test QueryService.ArchiveTrace() with no ArchiveSpanWriter.
func TestArchiveTraceNoOptions(t *testing.T) {
	qs, _, _ := initializeTestService()

	type contextKey string
	ctx := context.Background()
	err := qs.ArchiveTrace(context.WithValue(ctx, contextKey("foo"), "bar"), mockTraceID)
	assert.Equal(t, errNoArchiveSpanStorage, err)
}

// Test QueryService.ArchiveTrace() with ArchiveSpanWriter but invalid traceID.
func TestArchiveTraceWithInvalidTraceID(t *testing.T) {
	qs, readMock, _, readArchiveMock, _ := initializeTestServiceWithArchiveOptions()
	readMock.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(nil, spanstore.ErrTraceNotFound).Once()
	readArchiveMock.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(nil, spanstore.ErrTraceNotFound).Once()

	type contextKey string
	ctx := context.Background()
	err := qs.ArchiveTrace(context.WithValue(ctx, contextKey("foo"), "bar"), mockTraceID)
	assert.Equal(t, spanstore.ErrTraceNotFound, err)
}

// Test QueryService.ArchiveTrace(), save error with ArchiveSpanWriter.
func TestArchiveTraceWithArchiveWriterError(t *testing.T) {
	qs, readMock, _, _, writeMock := initializeTestServiceWithArchiveOptions()
	readMock.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(mockTrace, nil).Once()
	writeMock.On("WriteSpan", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*model.Span")).
		Return(errors.New("cannot save")).Times(2)

	type contextKey string
	ctx := context.Background()
	multiErr := qs.ArchiveTrace(context.WithValue(ctx, contextKey("foo"), "bar"), mockTraceID)
	assert.Len(t, multiErr, 2)
	// There are two spans in the mockTrace, ArchiveTrace should return a wrapped error.
	assert.EqualError(t, multiErr, "[cannot save, cannot save]")
}

// Test QueryService.ArchiveTrace() with correctly configured ArchiveSpanWriter.
func TestArchiveTraceSuccess(t *testing.T) {
	qs, readMock, _, _, writeMock := initializeTestServiceWithArchiveOptions()
	readMock.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(mockTrace, nil).Once()
	writeMock.On("WriteSpan", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("*model.Span")).
		Return(nil).Times(2)

	type contextKey string
	ctx := context.Background()
	err := qs.ArchiveTrace(context.WithValue(ctx, contextKey("foo"), "bar"), mockTraceID)
	assert.NoError(t, err)
}

// Test QueryService.Adjust()
func TestTraceAdjustmentFailure(t *testing.T) {
	qs := initializeTestServiceWithAdjustOption()

	_, err := qs.Adjust(mockTrace)
	assert.Error(t, err)
	assert.EqualValues(t, errAdjustment.Error(), err.Error())
}

// Test QueryService.GetDependencies()
func TestGetDependencies(t *testing.T) {
	qs, _, depsMock := initializeTestService()
	expectedDependencies := []model.DependencyLink{
		{
			Parent:    "killer",
			Child:     "queen",
			CallCount: 12,
		},
	}
	endTs := time.Unix(0, 1476374248550*millisToNanosMultiplier)
	depsMock.On("GetDependencies", endTs, defaultDependencyLookbackDuration).Return(expectedDependencies, nil).Times(1)

	actualDependencies, err := qs.GetDependencies(context.Background(), time.Unix(0, 1476374248550*millisToNanosMultiplier), defaultDependencyLookbackDuration)
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
