// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/model"
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
