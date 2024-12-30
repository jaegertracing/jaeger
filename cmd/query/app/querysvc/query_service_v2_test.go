package querysvc

import (
	"context"
	"testing"
	"time"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage_v2/depstore"
	depstoremocks "github.com/jaegertracing/jaeger/storage_v2/depstore/mocks"
	"github.com/jaegertracing/jaeger/storage_v2/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/storage_v2/tracestore/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
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

func initializeTestServiceV2(optionAppliers ...testOptionV2) *testQueryServiceV2 {
	traceReader := &tracestoremocks.Reader{}
	dependencyStorage := &depstoremocks.Reader{}

	options := QueryServiceOptionsV2{}

	tqs := testQueryServiceV2{
		traceReader: traceReader,
		depsReader:  dependencyStorage,
	}

	for _, optApplier := range optionAppliers {
		optApplier(&tqs, &options)
	}

	tqs.queryService = NewQueryServiceV2(traceReader, dependencyStorage, options)
	return &tqs
}

func TestGetServicesV2(t *testing.T) {
	tqs := initializeTestServiceV2()
	expectedServices := []string{"trifle", "bling"}
	tqs.traceReader.On("GetServices", mock.AnythingOfType("*context.valueCtx")).Return(expectedServices, nil).Once()

	type contextKey string
	ctx := context.Background()
	actualServices, err := tqs.queryService.GetServices(context.WithValue(ctx, contextKey("foo"), "bar"))
	require.NoError(t, err)
	assert.Equal(t, expectedServices, actualServices)
}

func TestGetOperationsV2(t *testing.T) {
	tqs := initializeTestServiceV2()
	expectedOperations := []tracestore.Operation{{Name: "", SpanKind: ""}, {Name: "get", SpanKind: ""}}
	operationQuery := tracestore.OperationQueryParameters{ServiceName: "abc/trifle"}
	tqs.traceReader.On(
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

func TestGetCapabilitiesV2(t *testing.T) {
	tqs := initializeTestServiceV2()
	expectedStorageCapabilities := StorageCapabilities{
		ArchiveStorage: false,
	}
	assert.Equal(t, expectedStorageCapabilities, tqs.queryService.GetCapabilities())
}

func TestGetCapabilitiesWithSupportsArchiveV2(t *testing.T) {
	tqs := initializeTestServiceV2(withArchiveTraceReader(), withArchiveTraceWriter())

	expectedStorageCapabilities := StorageCapabilities{
		ArchiveStorage: true,
	}
	assert.Equal(t, expectedStorageCapabilities, tqs.queryService.GetCapabilities())
}
