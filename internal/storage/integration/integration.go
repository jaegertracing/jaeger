// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
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
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
	samplemodel "github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore/model"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

//go:embed fixtures
var fixtures embed.FS

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

	// TODO: remove this after all storage backends return spanKind from GetOperations
	GetOperationsMissingSpanKind bool

	// TODO: remove this after all storage backends return Source column from GetDependencies

	GetDependenciesReturnsSource bool

	// List of tests which has to be skipped, it can be regex too.
	SkipList []string

	// CleanUp() should ensure that the storage backend is clean before another test.
	// called either before or after each test, and should be idempotent
	CleanUp func(t *testing.T)
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
			attributes.PutDouble(k, v)
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

func SkipUnlessEnv(t *testing.T, storage ...string) {
	env := os.Getenv("STORAGE")
	if slices.Contains(storage, env) {
		return
	}
	t.Skipf("This test requires environment variable STORAGE=%s", strings.Join(storage, "|"))
}

var CassandraSkippedTests = []string{
	"Tags_+_Operation_name_+_Duration_range",
	"Tags_+_Duration_range",
	"Tags_+_Operation_name_+_max_Duration",
	"Tags_+_max_Duration",
	"Operation_name_+_Duration_range",
	"Duration_range",
	"max_Duration",
	"Multiple_Traces",
}

func (s *StorageIntegration) skipIfNeeded(t *testing.T) {
	for _, pat := range s.SkipList {
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
	defer s.cleanUp(t)

	expected := []string{"example-service-1", "example-service-2", "example-service-3"}
	s.loadParseAndWriteExampleTrace(t)

	var actual []string
	found := s.waitForCondition(t, func(t *testing.T) bool {
		var err error
		actual, err = s.TraceReader.GetServices(context.Background())
		if err != nil {
			t.Log(err)
			return false
		}
		sort.Strings(actual)
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

func (s *StorageIntegration) helperTestGetTrace(
	t *testing.T,
	traceSize int,
	duplicateCount int,
	testName string,
	validator func(t *testing.T, actual ptrace.Traces),
) {
	s.skipIfNeeded(t)
	defer s.cleanUp(t)

	t.Logf("Testing %s...", testName)

	expected := s.writeLargeTraceWithDuplicateSpanIds(t, traceSize, duplicateCount)
	expectedTraceID := expected.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).TraceID()

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
	s.helperTestGetTrace(t, 10008, 0, "Large Trace over 10K without duplicates", nil)
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
	s.helperTestGetTrace(t, 200, 20, "Trace with duplicate span IDs", validator)
}

func (s *StorageIntegration) testGetOperations(t *testing.T) {
	s.skipIfNeeded(t)
	defer s.cleanUp(t)

	var expected []tracestore.Operation
	if s.GetOperationsMissingSpanKind {
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
	s.loadParseAndWriteExampleTrace(t)

	var actual []tracestore.Operation
	found := s.waitForCondition(t, func(t *testing.T) bool {
		var err error
		actual, err = s.TraceReader.GetOperations(context.Background(),
			tracestore.OperationQueryParams{ServiceName: "example-service-1"})
		if err != nil {
			t.Log(err)
			return false
		}
		sort.Slice(actual, func(i, j int) bool {
			return actual[i].Name < actual[j].Name
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
	defer s.cleanUp(t)

	expected := s.loadParseAndWriteExampleTrace(t)
	expectedTraceID := expected.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).TraceID()

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
		fakeTraceID := pcommon.TraceID{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
		iterTraces := s.TraceReader.GetTraces(context.Background(), tracestore.GetTraceParams{TraceID: fakeTraceID})
		traces, err := jiter.CollectWithErrors(jptrace.AggregateTraces(iterTraces))
		require.NoError(t, err) // v2 TraceReader no longer returns an error for not found
		assert.Empty(t, traces)
	})
}

func (s *StorageIntegration) testFindTraces(t *testing.T) {
	s.skipIfNeeded(t)
	defer s.cleanUp(t)

	// Note: all cases include ServiceName + StartTime range
	s.Fixtures = append(s.Fixtures, LoadAndParseQueryTestCases(t, "fixtures/queries.json")...)

	// Each query test case only specifies matching traces, but does not provide counterexamples.
	// To improve coverage we get all possible traces and store all of them before running queries.
	allTraceFixtures := make(map[string]ptrace.Traces)
	expectedTracesPerTestCase := make([][]ptrace.Traces, 0, len(s.Fixtures))
	for _, queryTestCase := range s.Fixtures {
		var expected []ptrace.Traces
		for _, traceFixture := range queryTestCase.ExpectedFixtures {
			trace, ok := allTraceFixtures[traceFixture]
			if !ok {
				trace = s.getTraceFixture(t, traceFixture)
				s.writeTrace(t, trace)
				allTraceFixtures[traceFixture] = trace
			}
			expected = append(expected, trace)
		}
		expectedTracesPerTestCase = append(expectedTracesPerTestCase, expected)
	}
	for i, queryTestCase := range s.Fixtures {
		t.Run(queryTestCase.Caption, func(t *testing.T) {
			s.skipIfNeeded(t)
			expected := expectedTracesPerTestCase[i]
			actual := s.findTracesByQuery(t, queryTestCase.Query.ToTraceQueryParams(t), expected)
			CompareTraceSlices(t, expected, actual)
		})
	}
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
	t.Logf("%-23s Writing trace with %d spans", time.Now().Format("2006-01-02 15:04:05.999"), trace.SpanCount())
	ctx, cx := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cx()
	err := s.TraceWriter.WriteTraces(ctx, trace)
	require.NoError(t, err, "Not expecting error when writing trace to storage")

	t.Logf("%-23s Finished writing trace with %d spans", time.Now().Format("2006-01-02 15:04:05.999"), trace.SpanCount())
}

func (s *StorageIntegration) loadParseAndWriteExampleTrace(t *testing.T) ptrace.Traces {
	trace := s.getTraceFixture(t, "example_trace")
	s.writeTrace(t, trace)
	return trace
}

func (s *StorageIntegration) writeLargeTraceWithDuplicateSpanIds(
	t *testing.T,
	totalCount int,
	dupFreq int,
) ptrace.Traces {
	trace := s.getTraceFixture(t, "example_trace")
	repeatedResourceSpan := trace.ResourceSpans().At(0)
	repeatedScopeSpan := repeatedResourceSpan.ScopeSpans().At(0)
	repeatedSpans := repeatedScopeSpan.Spans()
	repeatedSpan := repeatedSpans.At(0)
	newResourceSpan := ptrace.NewResourceSpans()
	newScopeSpan := newResourceSpan.ScopeSpans().AppendEmpty()
	repeatedResourceSpan.Resource().CopyTo(newResourceSpan.Resource())
	repeatedScopeSpan.Scope().CopyTo(newScopeSpan.Scope())
	newSpans := newScopeSpan.Spans()
	newSpans.EnsureCapacity(totalCount)
	for i := range totalCount {
		newSpan := ptrace.NewSpan()
		repeatedSpan.CopyTo(newSpan)
		switch {
		case dupFreq > 0 && i > 0 && i%dupFreq == 0:
			newSpan.SetSpanID(repeatedSpan.SpanID())
		default:
			var spanId [8]byte
			//nolint:gosec // G115 // i is always positive
			binary.BigEndian.PutUint64(spanId[:], uint64(i+1))
			newSpan.SetSpanID(spanId)
		}
		newSpan.SetStartTimestamp(pcommon.NewTimestampFromTime(newSpan.StartTimestamp().AsTime().Add(time.Second * time.Duration(i+1))))
		newSpan.SetEndTimestamp(pcommon.NewTimestampFromTime(newSpan.EndTimestamp().AsTime().Add(time.Second * time.Duration(i+1))))
		newSpan.CopyTo(newSpans.AppendEmpty())
	}
	newTrace := ptrace.NewTraces()
	newResourceSpan.CopyTo(newTrace.ResourceSpans().AppendEmpty())
	s.writeTrace(t, newTrace)
	return newTrace
}

func (*StorageIntegration) getTraceFixture(t *testing.T, fixture string) ptrace.Traces {
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
	if !s.GetDependenciesReturnsSource {
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
	require.NoError(t, s.DependencyWriter.WriteDependencies(startTime, expected))

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
		sort.Slice(actual, func(i, j int) bool {
			return actual[i].Parent < actual[j].Parent
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

// RunSpanStoreTests runs only span related integration tests
func (s *StorageIntegration) RunSpanStoreTests(t *testing.T) {
	t.Run("GetServices", s.testGetServices)
	t.Run("GetOperations", s.testGetOperations)
	t.Run("GetTrace", s.testGetTrace)
	t.Run("GetLargeTrace", s.testGetLargeTrace)
	t.Run("GetTraceWithDuplicateSpans", s.testGetTraceWithDuplicates)
	t.Run("FindTraces", s.testFindTraces)
}
