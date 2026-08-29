// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"cmp"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
	samplemodel "github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore/model"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

//go:embed fixtures
var fixtures embed.FS

// StorageType is a typed string for the STORAGE environment variable
// to avoid typos in test skip guards.
type StorageType string

const (
	StorageElasticsearch StorageType = "elasticsearch"
	StorageOpenSearch    StorageType = "opensearch"
	StorageKafka         StorageType = "kafka"
	StorageGRPC          StorageType = "grpc"
	StorageBadger        StorageType = "badger"
	StorageCassandra     StorageType = "cassandra"
	StorageClickHouse    StorageType = "clickhouse"
	StorageQuery         StorageType = "query"

	// StorageMemory is used for direct-path memory tests (runs during `make cover`).
	// StorageMemoryV2 is used for e2e memory tests that require a pre-built binary.
	// They cannot be consolidated because `make cover` runs ./... and would trigger
	// the e2e test which expects the Jaeger binary to exist.
	StorageMemory   StorageType = "memory"
	StorageMemoryV2 StorageType = "memory_v2"
)

// StorageIntegration holds components for storage integration test.
// The intended usage is as follows:
// - a specific storage implementation declares its own test functions
// - in those functions it instantiates and populates this struct
// - it then calls RunAll.
//
// Some implementations may declare multiple tests, with different settings,
// and RunAll() under different conditions.
type StorageIntegration struct {
	TraceWriter      tracestore.Writer
	TraceReader      tracestore.Reader
	DependencyWriter depstore.Writer
	DependencyReader depstore.Reader
	SamplingStore    samplingstore.Store
	Fixtures         []*QueryFixtures
	Capabilities     capabilities.Capabilities

	// CleanUp() should ensure that the storage backend is clean before another test.
	// called either before or after each test, and should be idempotent
	CleanUp func(t *testing.T)

	// Corpus is the data WriteCorpus writes and AssertCorpus reads back. RunSpanStoreTests builds
	// it; a suite that runs the two phases against different Jaeger processes builds it once and
	// sets it on both, so that the reader compares against the fixture timestamps the writer wrote.
	Corpus *Corpus
}

// requireCorpus fails with the reason rather than a nil dereference somewhere further along.
func (s *StorageIntegration) requireCorpus(t *testing.T) {
	require.NotNil(t, s.Corpus, "Corpus must be provided, or built by RunSpanStoreTests")
}

// === SpanStore Integration Tests ===

type Query struct {
	ServiceName   string
	OperationName string
	Tags          map[string]any
	StartTimeMin  time.Time
	StartTimeMax  time.Time
	DurationMin   time.Duration
	DurationMax   time.Duration
	NumTraces     int
}

func (q *Query) ToTraceQueryParams(t *testing.T) *tracestore.TraceQueryParams {
	attributes := pcommon.NewMap()
	for k, v := range q.Tags {
		switch v := v.(type) {
		case string:
			attributes.PutStr(k, v)
		case int:
			attributes.PutInt(k, int64(v))
		case float64:
			// JSON numbers are always float64 in Go; detect integers.
			if v == math.Trunc(v) && !math.IsInf(v, 0) && !math.IsNaN(v) {
				attributes.PutInt(k, int64(v))
			} else {
				attributes.PutDouble(k, v)
			}
		case bool:
			attributes.PutBool(k, v)
		default:
			t.Fatalf("Unsupported tag value type: %T", v)
		}
	}

	return &tracestore.TraceQueryParams{
		ServiceName:   q.ServiceName,
		OperationName: q.OperationName,
		Attributes:    attributes,
		StartTimeMin:  q.StartTimeMin,
		StartTimeMax:  q.StartTimeMax,
		DurationMin:   q.DurationMin,
		DurationMax:   q.DurationMax,
		SearchDepth:   q.NumTraces,
	}
}

// QueryFixtures and TraceFixtures are under ./fixtures/queries.json and ./fixtures/traces/*.json respectively.
// Each query fixture includes:
// - Caption: describes the query we are testing
// - Query: the query we are testing
// - ExpectedFixture: the trace fixture that we want back from these queries.
// Queries are not necessarily numbered, but since each query requires a service name,
// the service name is formatted "query##-service".
type QueryFixtures struct {
	Caption          string
	Query            *Query
	ExpectedFixtures []string
}

func (s *StorageIntegration) cleanUp(t *testing.T) {
	require.NotNil(t, s.CleanUp, "CleanUp function must be provided")
	s.CleanUp(t)
}

func SkipUnlessEnv(t *testing.T, storage ...StorageType) {
	if !capabilities.IsBackwardCompatibilityEnv() && strings.Contains(t.Name(), "BackwardCompatibility") {
		t.Skip("This test requires capability backward-compatibility environment")
	}
	env := os.Getenv("STORAGE")
	for _, s := range storage {
		if string(s) == env {
			return
		}
	}
	names := make([]string, len(storage))
	for i, s := range storage {
		names[i] = string(s)
	}
	t.Skipf("This test requires environment variable STORAGE=%s", strings.Join(names, "|"))
}

func (s *StorageIntegration) skipReadingTracesIfNeeded(t *testing.T) {
	if s.SkipReadingTraces {
		t.Skip()
	}
}

func (s *StorageIntegration) skipIfNeeded(t *testing.T) {
	for _, pat := range s.Capabilities.SkipList() {
		escapedPat := regexp.QuoteMeta(pat)
		ok, err := regexp.MatchString(escapedPat, t.Name())
		require.NoError(t, err)
		if ok {
			t.Skip()
			return
		}
	}
}

func (*StorageIntegration) waitForCondition(t *testing.T, predicate func(t *testing.T) bool) bool {
	const iterations = 100 // Will wait at most 100 seconds.
	for i := range iterations {
		if predicate(t) {
			return true
		}
		t.Logf("Waiting for storage backend to update documents, iteration %d out of %d", i+1, iterations)
		time.Sleep(time.Second)
	}
	return predicate(t)
}

func (s *StorageIntegration) testGetServices(t *testing.T) {
	s.skipIfNeeded(t)

	expected := s.Corpus.Services()

	var actual []string
	found := s.waitForCondition(t, func(t *testing.T) bool {
		var err error
		actual, err = s.TraceReader.GetServices(context.Background())
		if err != nil {
			t.Log(err)
			return false
		}
		slices.Sort(actual)
		t.Logf("Retrieved services: %v", actual)
		if len(actual) > len(expected) {
			// If the storage backend returns more services than expected, let's log traces for those
			t.Log("🛑 Found unexpected services!")
			for _, service := range actual {
				iterTraces := s.TraceReader.FindTraces(context.Background(), tracestore.TraceQueryParams{
					ServiceName:  service,
					StartTimeMin: time.Now().Add(-2 * time.Hour),
					StartTimeMax: time.Now(),
				})
				for traces, err := range iterTraces {
					if err != nil {
						t.Log(err)
						continue
					}
					for _, trace := range traces {
						for _, span := range jptrace.SpanIter(trace) {
							t.Logf("span: Service: %s, TraceID: %s, Operation: %s", service, span.TraceID(), span.Name())
						}
					}
				}
			}
		}
		return assert.ObjectsAreEqualValues(expected, actual)
	})

	if !assert.True(t, found) {
		t.Log("\t Expected:", expected)
		t.Log("\t Actual  :", actual)
	}
}

// assertTraceByID reads one trace of the corpus back by ID and requires the backend to return at
// least every span that was written.
func (s *StorageIntegration) assertTraceByID(
	t *testing.T,
	expected ptrace.Traces,
	validator func(t *testing.T, actual ptrace.Traces),
) {
	s.skipIfNeeded(t)

	expectedTraceID := jptrace.GetTraceID(expected)

	actual := ptrace.NewTraces()
	found := s.waitForCondition(t, func(_ *testing.T) bool {
		iterTraces := s.TraceReader.GetTraces(context.Background(), tracestore.GetTraceParams{TraceID: expectedTraceID})
		traces, err := jiter.CollectWithErrors(jptrace.AggregateTraces(iterTraces))
		if err != nil {
			t.Logf("Error loading trace: %v", err)
			return false
		}
		if len(traces) == 0 {
			return false
		}
		require.Len(t, traces, 1)
		actual = traces[0]
		return actual.SpanCount() >= expected.SpanCount()
	})

	t.Logf("%-23s Loaded trace, expected=%d, actual=%d", time.Now().Format("2006-01-02 15:04:05.999"), expected.SpanCount(), actual.SpanCount())
	if !assert.True(t, found, "error loading trace, expected=%d, actual=%d", expected.SpanCount(), actual.SpanCount()) {
		CompareTraces(t, expected, actual)
		return
	}

	if validator != nil {
		validator(t, actual)
	}
}

func (s *StorageIntegration) testGetLargeTrace(t *testing.T) {
	s.assertTraceByID(t, s.Corpus.Large, nil)
}

func (s *StorageIntegration) testGetTraceWithDuplicates(t *testing.T) {
	validator := func(t *testing.T, actual ptrace.Traces) {
		duplicateCount := 0
		seenIDs := make(map[pcommon.SpanID]int)
		for _, span := range jptrace.SpanIter(actual) {
			seenIDs[span.SpanID()]++
			if seenIDs[span.SpanID()] > 1 {
				duplicateCount++
			}
		}
		assert.Positive(t, duplicateCount, "Duplicate SpanIDs should be present in the trace")
	}
	s.assertTraceByID(t, s.Corpus.Duplicates, validator)
}

func (s *StorageIntegration) testGetOperations(t *testing.T) {
	s.skipIfNeeded(t)

	var expected []tracestore.Operation
	if s.Capabilities.GetOperationsMissingSpanKind() {
		expected = []tracestore.Operation{
			{Name: "example-operation-1"},
			{Name: "example-operation-3"},
			{Name: "example-operation-4"},
		}
	} else {
		expected = []tracestore.Operation{
			{Name: "example-operation-1", SpanKind: ""},
			{Name: "example-operation-3", SpanKind: "server"},
			{Name: "example-operation-4", SpanKind: "client"},
		}
	}

	var actual []tracestore.Operation
	found := s.waitForCondition(t, func(t *testing.T) bool {
		var err error
		actual, err = s.TraceReader.GetOperations(context.Background(),
			tracestore.OperationQueryParams{ServiceName: "example-service-1"})
		if err != nil {
			t.Log(err)
			return false
		}
		slices.SortFunc(actual, func(a, b tracestore.Operation) int {
			return cmp.Compare(a.Name, b.Name)
		})
		t.Logf("Retrieved operations: %v", actual)
		return assert.ObjectsAreEqualValues(expected, actual)
	})

	if !assert.True(t, found) {
		t.Log("\t Expected:", expected)
		t.Log("\t Actual  :", actual)
	}
}

func (s *StorageIntegration) testGetTrace(t *testing.T) {
	s.skipIfNeeded(t)

	expected := s.Corpus.Example
	expectedTraceID := jptrace.GetTraceID(expected)

	actual := ptrace.Traces{} // no spans
	found := s.waitForCondition(t, func(t *testing.T) bool {
		iterTraces := s.TraceReader.GetTraces(context.Background(), tracestore.GetTraceParams{TraceID: expectedTraceID})
		traces, err := jiter.CollectWithErrors(jptrace.AggregateTraces(iterTraces))
		if err != nil {
			t.Log(err)
			return false
		}
		if len(traces) == 0 {
			return false
		}
		require.Len(t, traces, 1)
		actual = traces[0]
		return actual.SpanCount() >= expected.SpanCount()
	})
	if !assert.True(t, found) {
		CompareTraces(t, expected, actual)
	}

	t.Run("NotFound error", func(t *testing.T) {
		fakeTraceID := s.Corpus.AbsentTraceID(t)
		iterTraces := s.TraceReader.GetTraces(context.Background(), tracestore.GetTraceParams{TraceID: fakeTraceID})
		traces, err := jiter.CollectWithErrors(jptrace.AggregateTraces(iterTraces))
		require.NoError(t, err) // v2 TraceReader no longer returns an error for not found
		assert.Empty(t, traces)
	})
}

func (s *StorageIntegration) testFindTraces(t *testing.T) {
	s.skipIfNeeded(t)

	expectations := s.Corpus.QueryExpectations(t)
	for i, queryTestCase := range s.Corpus.Queries {
		t.Run(queryTestCase.Caption, func(t *testing.T) {
			s.skipIfNeeded(t)
			expected := expectations[i]
			actual := s.findTracesByQuery(t, queryTestCase.Query.ToTraceQueryParams(t), expected)
			CompareTraceSlices(t, expected, actual)
		})
	}
}

func (s *StorageIntegration) testFindTraceSummaries(t *testing.T) {
	s.skipIfNeeded(t)

	trace := s.Corpus.Example

	// Derive the expected trace ID, time range, and service name from the written trace.
	expectedTraceID := jptrace.GetTraceID(trace)
	var minStart, maxEnd time.Time
	var serviceName string
	for pos, span := range jptrace.SpanIter(trace) {
		start := span.StartTimestamp().AsTime()
		end := span.EndTimestamp().AsTime()
		if minStart.IsZero() || start.Before(minStart) {
			minStart = start
		}
		if maxEnd.IsZero() || end.After(maxEnd) {
			maxEnd = end
		}
		if serviceName == "" {
			if v, ok := pos.Resource.Resource().Attributes().Get("service.name"); ok {
				serviceName = v.Str()
			}
		}
	}

	require.NotEmpty(t, serviceName, "service name must be present in trace fixture")
	require.False(t, minStart.IsZero(), "min start time must be present in trace fixture")
	require.False(t, maxEnd.IsZero(), "max end time must be present in trace fixture")

	query := tracestore.TraceQueryParams{
		ServiceName:  serviceName,
		Attributes:   pcommon.NewMap(),
		StartTimeMin: minStart.Add(-time.Minute),
		StartTimeMax: maxEnd.Add(time.Minute),
		SearchDepth:  10,
	}

	expectedSpanCount := trace.SpanCount()
	var summary *tracestore.TraceSummary
	found := s.waitForCondition(t, func(t *testing.T) bool {
		batches, err := jiter.CollectWithErrors(s.TraceReader.FindTraceSummaries(context.Background(), query))
		if err != nil {
			t.Log(err)
			return false
		}
		summary = nil
		for _, b := range batches {
			for i := range b {
				if b[i].TraceID == expectedTraceID {
					sm := b[i]
					summary = &sm
				}
			}
		}
		// ES refreshes asynchronously, so an early query can observe a partially
		// indexed trace. Wait until the summary reports the full span count.
		return summary != nil && summary.SpanCount == expectedSpanCount
	})
	require.True(t, found, "timed out waiting for the complete FindTraceSummaries result for trace %s", expectedTraceID)
	require.NotNil(t, summary)

	assert.Equal(t, expectedSpanCount, summary.SpanCount)
	assert.False(t, summary.MinStartTime.IsZero(), "MinStartTime should not be zero")
	assert.False(t, summary.MaxEndTime.IsZero(), "MaxEndTime should not be zero")
	assert.NotEmpty(t, summary.Services, "services should not be empty")
}

// testFindTracesWithoutServiceName is the cross-service search RFC 0013 exists for: two
// traces from different services share an attribute, and one query carrying only that
// attribute and a time range returns both. It is the assertion the httptest snapshots
// cannot make — that the backend really reads an absent service name as "any service".
//
// The gate is the suite's per-backend opt-out, which CI populates from the STORAGE under
// test, not the reader's own SearchCapabilities: in the e2e configuration that reader talks
// to a query service over api_v3, which cannot report capabilities, so gating on it would
// skip everywhere (RFC 0013 §3.7).
func (s *StorageIntegration) testFindTracesWithoutServiceName(t *testing.T) {
	s.skipIfNeeded(t)
	if s.Capabilities.SearchRequiresServiceName() {
		t.Skip("this storage backend requires a service name to search")
	}

	expected := s.Corpus.CrossService

	attributes := pcommon.NewMap()
	attributes.PutStr(crossServiceMarker, "yes")
	query := &tracestore.TraceQueryParams{
		Attributes:   attributes,
		StartTimeMin: time.Now().Add(-time.Hour),
		StartTimeMax: time.Now().Add(time.Hour),
		SearchDepth:  10,
	}

	actual := s.findTracesByQuery(t, query, expected)

	services := make([]string, 0, len(actual))
	for _, trace := range actual {
		for i := 0; i < trace.ResourceSpans().Len(); i++ {
			if name, ok := trace.ResourceSpans().At(i).Resource().Attributes().Get(otelsemconv.ServiceNameKey); ok {
				services = append(services, name.Str())
			}
		}
	}
	slices.Sort(services)
	assert.Equal(t, []string{"cross-service-a", "cross-service-b"}, services,
		"a search with no service name must return the traces of both services")
}

func (s *StorageIntegration) findTracesByQuery(t *testing.T, query *tracestore.TraceQueryParams, expected []ptrace.Traces) []ptrace.Traces {
	var traces []ptrace.Traces
	found := s.waitForCondition(t, func(t *testing.T) bool {
		iterTraces := s.TraceReader.FindTraces(context.Background(), *query)
		var err error
		traces, err = jiter.CollectWithErrors(jptrace.AggregateTraces(iterTraces))
		if err != nil {
			t.Log(err)
			return false
		}
		if len(expected) != len(traces) {
			t.Logf("Expecting certain number of traces: expected: %d, actual: %d", len(expected), len(traces))
			return false
		}

		if spanCount(expected) != spanCount(traces) {
			t.Logf("Expecting certain number of spans: expected: %d, actual: %d", spanCount(expected), spanCount(traces))
			return false
		}
		return true
	})
	require.True(t, found)
	return traces
}

func (s *StorageIntegration) writeTrace(t *testing.T, trace ptrace.Traces) {
	if s.SkipWritingTraces {
		return
	}
	t.Logf("%-23s Writing trace with %d spans", time.Now().Format("2006-01-02 15:04:05.999"), trace.SpanCount())
	ctx, cx := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cx()
	err := s.TraceWriter.WriteTraces(ctx, trace)
	require.NoError(t, err, "Not expecting error when writing trace to storage")

	t.Logf("%-23s Finished writing trace with %d spans", time.Now().Format("2006-01-02 15:04:05.999"), trace.SpanCount())
}

func getTraceFixture(t *testing.T, fixture string) ptrace.Traces {
	fileName := fmt.Sprintf("fixtures/traces/%s.json", fixture)
	return loadOTLPTrace(t, fileName)
}

func loadOTLPTrace(t *testing.T, fileName string) ptrace.Traces {
	// #nosec
	inStr, err := fixtures.ReadFile(fileName)
	require.NoError(t, err, "Not expecting error when loading fixture %s", fileName)
	unmarshaller := ptrace.JSONUnmarshaler{}
	td, err := unmarshaller.UnmarshalTraces(inStr)
	require.NoError(t, err, "Not expecting error when unmarshaling fixture %s", fileName)
	correctTimeForTrace(td)
	return td
}

// LoadAndParseQueryTestCases loads and parses query test cases
func LoadAndParseQueryTestCases(t *testing.T, queriesFile string) []*QueryFixtures {
	var queries []*QueryFixtures
	loadAndParseJSON(t, queriesFile, &queries)
	return queries
}

func loadAndParseJSON(t *testing.T, path string, object any) {
	// #nosec
	inStr, err := fixtures.ReadFile(path)
	require.NoError(t, err, "Not expecting error when loading fixture %s", path)
	err = json.Unmarshal(correctTime(inStr), object)
	require.NoError(t, err, "Not expecting error when unmarshaling fixture %s", path)
}

// required, because we want to only query on recent traces, so we replace all the dates with recent dates.
func correctTime(jsonData []byte) []byte {
	jsonString := string(jsonData)
	now := time.Now().UTC()
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	twoDaysAgo := now.AddDate(0, 0, -2).Format("2006-01-02")
	retString := strings.ReplaceAll(jsonString, "2017-01-26", yesterday)
	retString = strings.ReplaceAll(retString, "2017-01-25", twoDaysAgo)
	return []byte(retString)
}

func correctTimeForTrace(td ptrace.Traces) {
	now := time.Now().UTC()
	normalizer := newDateOffsetNormalizer(now)
	normalizer.normalizeTrace(td)
}

func spanCount(traces []ptrace.Traces) int {
	var count int
	for _, trace := range traces {
		count += trace.SpanCount()
	}
	return count
}

// === DependencyStore Integration Tests ===

func (s *StorageIntegration) testGetDependencies(t *testing.T) {
	if s.DependencyReader == nil || s.DependencyWriter == nil {
		t.Skip("Skipping GetDependencies test because dependency reader or writer is nil")
		return
	}

	s.skipIfNeeded(t)
	defer s.cleanUp(t)

	source := model.JaegerDependencyLinkSource
	if !s.Capabilities.GetDependenciesMissingSource() {
		source = ""
	}

	expected := []model.DependencyLink{
		{
			Parent:    "hello",
			Child:     "world",
			CallCount: uint64(1),
			Source:    source,
		},
		{
			Parent:    "world",
			Child:     "hello",
			CallCount: uint64(3),
			Source:    source,
		},
	}
	startTime := time.Now()
	require.NoError(t, s.DependencyWriter.WriteDependencies(t.Context(), startTime, expected))

	var actual []model.DependencyLink
	found := s.waitForCondition(t, func(t *testing.T) bool {
		var err error

		actual, err = s.DependencyReader.GetDependencies(
			context.Background(),
			depstore.QueryParameters{
				StartTime: startTime,
				EndTime:   startTime.Add(time.Minute * 5),
			},
		)
		if err != nil {
			t.Log(err)
			return false
		}
		slices.SortFunc(actual, func(a, b model.DependencyLink) int {
			return cmp.Compare(a.Parent, b.Parent)
		})
		return assert.ObjectsAreEqualValues(expected, actual)
	})

	if !assert.True(t, found) {
		t.Log("\t Expected:", expected)
		t.Log("\t Actual  :", actual)
	}
}

// === Sampling Store Integration Tests ===

func (s *StorageIntegration) testGetThroughput(t *testing.T) {
	s.skipIfNeeded(t)
	if s.SamplingStore == nil {
		t.Skip("Skipping GetThroughput test because sampling store is nil")
		return
	}
	defer s.cleanUp(t)
	start := time.Now()

	s.insertThroughput(t)

	expected := 2
	var actual []*samplemodel.Throughput
	_ = s.waitForCondition(t, func(t *testing.T) bool {
		var err error
		actual, err = s.SamplingStore.GetThroughput(start, start.Add(time.Second*time.Duration(10)))
		if err != nil {
			t.Log(err)
			return false
		}
		return assert.ObjectsAreEqualValues(expected, len(actual))
	})
	assert.Len(t, actual, expected)
}

func (s *StorageIntegration) testGetLatestProbability(t *testing.T) {
	s.skipIfNeeded(t)
	if s.SamplingStore == nil {
		t.Skip("Skipping GetLatestProbability test because sampling store is nil")
		return
	}
	defer s.cleanUp(t)

	s.SamplingStore.InsertProbabilitiesAndQPS("newhostname1", samplemodel.ServiceOperationProbabilities{"new-srv3": {"op": 0.123}}, samplemodel.ServiceOperationQPS{"new-srv2": {"op": 11}})
	s.SamplingStore.InsertProbabilitiesAndQPS("dell11eg843d", samplemodel.ServiceOperationProbabilities{"new-srv": {"op": 0.1}}, samplemodel.ServiceOperationQPS{"new-srv": {"op": 4}})

	expected := samplemodel.ServiceOperationProbabilities{"new-srv": {"op": 0.1}}
	var actual samplemodel.ServiceOperationProbabilities
	found := s.waitForCondition(t, func(t *testing.T) bool {
		var err error
		actual, err = s.SamplingStore.GetLatestProbabilities()
		if err != nil {
			t.Log(err)
			return false
		}
		return assert.ObjectsAreEqualValues(expected, actual)
	})
	if !assert.True(t, found) {
		t.Log("\t Expected:", expected)
		t.Log("\t Actual  :", actual)
	}
}

func (s *StorageIntegration) insertThroughput(t *testing.T) {
	throughputs := []*samplemodel.Throughput{
		{Service: "my-svc", Operation: "op"},
		{Service: "our-svc", Operation: "op2"},
	}
	err := s.SamplingStore.InsertThroughput(throughputs)
	require.NoError(t, err)
}

// RunAll runs all integration tests
func (s *StorageIntegration) RunAll(t *testing.T) {
	s.RunSpanStoreTests(t)
	t.Run("GetDependencies", s.testGetDependencies)
	t.Run("GetThroughput", s.testGetThroughput)
	t.Run("GetLatestProbability", s.testGetLatestProbability)
}

// RunSpanStoreTests runs only span related integration tests. It writes the corpus once and then
// only reads, so the backend holds every dataset for the whole run and each assertion has to
// discriminate its own data rather than rely on an empty store.
func (s *StorageIntegration) RunSpanStoreTests(t *testing.T) {
	s.Corpus = BuildCorpus(t, s.Fixtures, s.Capabilities)
	// Deferred rather than registered with t.Cleanup, because a suite may own resources that
	// CleanUp needs and tear them down in a defer of its own once this returns — the gRPC suite
	// closes its factory there, and a t.Cleanup would run after that and clean up through a closed
	// connection.
	defer s.cleanUp(t)
	s.WriteCorpus(t)
	s.AssertCorpus(t)
}

// AssertCorpus reads back a corpus that is already in the backend and writes nothing, so it can
// run against a different Jaeger process than the one that wrote it.
func (s *StorageIntegration) AssertCorpus(t *testing.T) {
	s.requireCorpus(t)
	t.Run("GetServices", s.testGetServices)
	t.Run("GetOperations", s.testGetOperations)
	t.Run("GetTrace", s.testGetTrace)
	t.Run("GetLargeTrace", s.testGetLargeTrace)
	t.Run("GetTraceWithDuplicateSpans", s.testGetTraceWithDuplicates)
	t.Run("FindTraces", s.testFindTraces)
	t.Run("FindTracesWithFilter", s.testFindTracesWithFilter)
	t.Run("FindTraceSummaries", s.testFindTraceSummaries)
	t.Run("FindTracesWithoutServiceName", s.testFindTracesWithoutServiceName)
}
