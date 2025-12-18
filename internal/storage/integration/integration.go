// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore"
	samplemodel "github.com/jaegertracing/jaeger/internal/storage/v1/api/samplingstore/model"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
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

func (q *Query) ToTraceQueryParams() *tracestore.TraceQueryParams {
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
	"OTLPScopeMetadata",
	"OTLPSpanLinks",
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
	for i := 0; i < iterations; i++ {
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
				traces, err := v1adapter.V1TracesFromSeq2(iterTraces)
				if err != nil {
					t.Log(err)
					continue
				}
				for _, trace := range traces {
					for _, span := range trace.Spans {
						t.Logf("span: Service: %s, TraceID: %s, Operation: %s", service, span.TraceID, span.OperationName)
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
	validator func(t *testing.T, actual *model.Trace),
) {
	s.skipIfNeeded(t)
	defer s.cleanUp(t)

	t.Logf("Testing %s...", testName)

	expected := s.writeLargeTraceWithDuplicateSpanIds(t, traceSize, duplicateCount)
	expectedTraceID := v1adapter.FromV1TraceID(expected.Spans[0].TraceID)

	actual := &model.Trace{} // no spans
	found := s.waitForCondition(t, func(_ *testing.T) bool {
		iterTraces := s.TraceReader.GetTraces(context.Background(), tracestore.GetTraceParams{TraceID: expectedTraceID})
		traces, err := v1adapter.V1TracesFromSeq2(iterTraces)
		if err != nil {
			t.Logf("Error loading trace: %v", err)
			return false
		}
		if len(traces) == 0 {
			return false
		}
		actual = traces[0]
		return len(actual.Spans) >= len(expected.Spans)
	})

	t.Logf("%-23s Loaded trace, expected=%d, actual=%d", time.Now().Format("2006-01-02 15:04:05.999"), len(expected.Spans), len(actual.Spans))
	if !assert.True(t, found, "error loading trace, expected=%d, actual=%d", len(expected.Spans), len(actual.Spans)) {
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
	validator := func(t *testing.T, actual *model.Trace) {
		duplicateCount := 0
		seenIDs := make(map[model.SpanID]int)

		for _, span := range actual.Spans {
			seenIDs[span.SpanID]++
			if seenIDs[span.SpanID] > 1 {
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

	// Subtest 1: Basic trace validation (works for all backends)
	t.Run("BasicTrace", func(t *testing.T) {
		expected := s.loadParseAndWriteExampleTrace(t)
		expectedTraceID := v1adapter.FromV1TraceID(expected.Spans[0].TraceID)

		actual := &model.Trace{}
		found := s.waitForCondition(t, func(t *testing.T) bool {
			iterTraces := s.TraceReader.GetTraces(context.Background(), tracestore.GetTraceParams{TraceID: expectedTraceID})
			traces, err := v1adapter.V1TracesFromSeq2(iterTraces)
			if err != nil {
				t.Log(err)
				return false
			}
			if len(traces) == 0 {
				return false
			}
			actual = traces[0]
			return len(actual.Spans) == len(expected.Spans)
		})
		if !assert.True(t, found) {
			CompareTraces(t, expected, actual)
		}

		t.Run("NotFound error", func(t *testing.T) {
			fakeTraceID := v1adapter.FromV1TraceID(model.TraceID{High: 0, Low: 1})
			iterTraces := s.TraceReader.GetTraces(context.Background(), tracestore.GetTraceParams{TraceID: fakeTraceID})
			traces, err := v1adapter.V1TracesFromSeq2(iterTraces)
			require.NoError(t, err)
			assert.Empty(t, traces)
		})
	})

	// Subtest 2: OTLP Scope metadata preservation (skip for Cassandra/ES)
	t.Run("OTLPScopeMetadata", func(t *testing.T) {
		s.skipIfNeeded(t)

		expectedTraces := loadOTLPFixture(t, "otlp_scope_attributes")
		traceID := extractTraceID(t, expectedTraces)
		s.writeTrace(t, expectedTraces)

		var retrievedTraces ptrace.Traces
		found := s.waitForCondition(t, func(t *testing.T) bool {
			iter := s.TraceReader.GetTraces(context.Background(), tracestore.GetTraceParams{TraceID: traceID})

			for trSlice, err := range iter {
				if err != nil {
					t.Logf("Error iterating traces: %v", err)
					return false
				}
				if len(trSlice) > 0 && trSlice[0].SpanCount() > 0 {
					retrievedTraces = trSlice[0]
					return true
				}
			}
			return false
		})

		require.True(t, found, "Failed to retrieve OTLP trace")
		require.Positive(t, retrievedTraces.ResourceSpans().Len(), "Should have resource spans")

		scopeSpans := retrievedTraces.ResourceSpans().At(0).ScopeSpans()
		require.Positive(t, scopeSpans.Len(), "Should have scope spans")

		scope := scopeSpans.At(0).Scope()

		assert.Equal(t, "test-instrumentation-library", scope.Name(), "Scope name should be preserved")
		assert.Equal(t, "2.1.0", scope.Version(), "Scope version should be preserved")

		scopeAttrs := scope.Attributes()
		assert.Positive(t, scopeAttrs.Len(), "Scope should have attributes")

		val, exists := scopeAttrs.Get("scope.attribute.key")
		assert.True(t, exists, "Scope attribute 'scope.attribute.key' should exist")
		assert.Equal(t, "scope-value", val.Str(), "Scope attribute value should match")

		t.Log("OTLP InstrumentationScope metadata and attributes preserved successfully")
	})

	// Subtest 3: OTLP Span Links with attributes (skip for Cassandra/ES)
	t.Run("OTLPSpanLinks", func(t *testing.T) {
		s.skipIfNeeded(t)

		expectedTraces := loadOTLPFixture(t, "otlp_span_links")
		traceID := extractTraceID(t, expectedTraces)
		s.writeTrace(t, expectedTraces)

		var retrievedTraces ptrace.Traces
		found := s.waitForCondition(t, func(t *testing.T) bool {
			iter := s.TraceReader.GetTraces(context.Background(), tracestore.GetTraceParams{TraceID: traceID})

			for trSlice, err := range iter {
				if err != nil {
					t.Logf("Error iterating traces: %v", err)
					return false
				}
				if len(trSlice) > 0 && trSlice[0].SpanCount() > 0 {
					retrievedTraces = trSlice[0]
					return true
				}
			}
			return false
		})

		require.True(t, found, "Failed to retrieve OTLP trace")
		require.Positive(t, retrievedTraces.ResourceSpans().Len(), "Should have resource spans")

		span := retrievedTraces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
		links := span.Links()

		require.Positive(t, links.Len(), "Span should have links")

		for i := 0; i < links.Len(); i++ {
			link := links.At(i)
			linkAttrs := link.Attributes()
			assert.Positive(t, linkAttrs.Len(), "Link should have attributes")

			val, exists := linkAttrs.Get("link.attribute.key")
			assert.True(t, exists, "Link attribute 'link.attribute.key' should exist")
			assert.Equal(t, "link-value", val.Str(), "Link attribute value should match")
		}

		t.Logf("OTLP span links with attributes preserved successfully: %d links", links.Len())
	})
}

func (s *StorageIntegration) testFindTraces(t *testing.T) {
	s.skipIfNeeded(t)
	defer s.cleanUp(t)

	// Note: all cases include ServiceName + StartTime range
	s.Fixtures = append(s.Fixtures, LoadAndParseQueryTestCases(t, "fixtures/queries.json")...)

	// Each query test case only specifies matching traces, but does not provide counterexamples.
	// To improve coverage we get all possible traces and store all of them before running queries.
	allTraceFixtures := make(map[string]*model.Trace)
	expectedTracesPerTestCase := make([][]*model.Trace, 0, len(s.Fixtures))
	for _, queryTestCase := range s.Fixtures {
		var expected []*model.Trace
		for _, traceFixture := range queryTestCase.ExpectedFixtures {
			trace, ok := allTraceFixtures[traceFixture]
			if !ok {
				otelTraces := s.getTraceFixture(t, traceFixture)
				s.writeTrace(t, otelTraces)
				trace = s.getTraceFixtureV1(t, traceFixture)
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
			actual := s.findTracesByQuery(t, queryTestCase.Query.ToTraceQueryParams(), expected)
			CompareSliceOfTraces(t, expected, actual)
		})
	}
}

func (s *StorageIntegration) findTracesByQuery(t *testing.T, query *tracestore.TraceQueryParams, expected []*model.Trace) []*model.Trace {
	var traces []*model.Trace
	found := s.waitForCondition(t, func(t *testing.T) bool {
		var err error
		iterTraces := s.TraceReader.FindTraces(context.Background(), *query)
		traces, err = v1adapter.V1TracesFromSeq2(iterTraces)
		if err != nil {
			t.Log(err)
			return false
		}
		if len(expected) != len(traces) {
			t.Logf("Expecting certain number of traces: expected: %d, actual: %d", len(expected), len(traces))
			return false
		}
		if spanCount(expected) != spanCount(traces) {
			t.Logf("Excepting certain number of spans: expected: %d, actual: %d", spanCount(expected), spanCount(traces))
			return false
		}
		return true
	})
	require.True(t, found)
	return traces
}

func (s *StorageIntegration) writeTrace(t *testing.T, traces ptrace.Traces) {
	spanCount := traces.SpanCount()
	t.Logf("%-23s Writing trace with %d spans", time.Now().Format("2006-01-02 15:04:05.999"), spanCount)
	ctx, cx := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cx()
	err := s.TraceWriter.WriteTraces(ctx, traces)
	require.NoError(t, err, "Not expecting error when writing trace to storage")

	t.Logf("%-23s Finished writing trace with %d spans", time.Now().Format("2006-01-02 15:04:05.999"), spanCount)
}

func (s *StorageIntegration) loadParseAndWriteExampleTrace(t *testing.T) *model.Trace {
	otelTraces := s.getTraceFixture(t, "example_trace")
	s.writeTrace(t, otelTraces)
	return s.getTraceFixtureV1(t, "example_trace")
}

func (s *StorageIntegration) writeLargeTraceWithDuplicateSpanIds(
	t *testing.T,
	totalCount int,
	dupFreq int,
) *model.Trace {
	trace := s.getTraceFixtureV1(t, "example_trace")
	repeatedSpan := trace.Spans[0]
	trace.Spans = make([]*model.Span, totalCount)
	for i := range totalCount {
		newSpan := new(model.Span)
		*newSpan = *repeatedSpan
		switch {
		case dupFreq > 0 && i > 0 && i%dupFreq == 0:
			newSpan.SpanID = repeatedSpan.SpanID
		default:
			newSpan.SpanID = model.SpanID(uint64(i) + 1) //nolint:gosec // G115
		}
		newSpan.StartTime = newSpan.StartTime.Add(time.Second * time.Duration(i+1))
		trace.Spans[i] = newSpan
	}
	// Convert to OTLP for writing
	otelTraces := v1adapter.V1TraceToOtelTrace(trace)
	s.writeTrace(t, otelTraces)
	return trace
}

// getTraceFixture returns OTLP traces ready for v2 API
func (s *StorageIntegration) getTraceFixture(t *testing.T, fixture string) ptrace.Traces {
    return loadOTLPFixture(t, fixture)
}

// getTraceFixtureV1 returns v1 model.Trace for comparison purposes
func (s *StorageIntegration) getTraceFixtureV1(t *testing.T, fixture string) *model.Trace {
    // Load OTLP fixture
    otelTraces := loadOTLPFixture(t, fixture)
    
    // Create an iterator that yields the single trace
    iter := func(yield func([]ptrace.Traces, error) bool) {
        yield([]ptrace.Traces{otelTraces}, nil)
    }
    
    // Use V1TracesFromSeq2 to convert
    traces, err := v1adapter.V1TracesFromSeq2(iter)
    require.NoError(t, err, "Failed to convert OTLP to v1 trace")
    require.Len(t, traces, 1, "Expected exactly one trace in fixture")
    
    return traces[0]
}


func getTraceFixtureExact(t *testing.T, fileName string) *model.Trace {
	var trace model.Trace
	loadAndParseJSONPB(t, fileName, &trace)
	return &trace
}

func loadAndParseJSONPB(t *testing.T, path string, object proto.Message) {
	// #nosec
	inStr, err := fixtures.ReadFile(path)
	require.NoError(t, err, "Not expecting error when loading fixture %s", path)
	err = jsonpb.Unmarshal(bytes.NewReader(correctTime(inStr)), object)
	require.NoError(t, err, "Not expecting error when unmarshaling fixture %s", path)
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

func spanCount(traces []*model.Trace) int {
	var count int
	for _, trace := range traces {
		count += len(trace.Spans)
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

// loadOTLPFixture loads an OTLP trace fixture by name from the fixtures directory.
func loadOTLPFixture(t *testing.T, fixtureName string) ptrace.Traces {
	fileName := fmt.Sprintf("fixtures/traces/%s.json", fixtureName)
	data, err := fixtures.ReadFile(fileName)
	require.NoError(t, err, "Failed to read OTLP fixture %s", fileName)

	unmarshaler := &ptrace.JSONUnmarshaler{}
	traces, err := unmarshaler.UnmarshalTraces(data)
	require.NoError(t, err, "Failed to unmarshal OTLP fixture %s", fixtureName)

	normalizeOTLPTimestamps(traces)

	return traces
}

// normalizeOTLPTimestamps adjusts all span timestamps in the trace to be recent.
// This ensures test queries with time ranges work correctly regardless of when the test runs.
func normalizeOTLPTimestamps(traces ptrace.Traces) {
	resourceSpans := traces.ResourceSpans()
	if resourceSpans.Len() == 0 {
		return
	}

	var firstStart time.Time
	targetStart := time.Now().Add(-time.Minute).UTC()

	// Use OTLP iterator functions to traverse and adjust timestamps
	for _, rs := range resourceSpans.All() {
		for _, ss := range rs.ScopeSpans().All() {
			for _, span := range ss.Spans().All() {
				// Detect first timestamp if not yet found
				if firstStart.IsZero() {
					firstStart = span.StartTimestamp().AsTime()
					if firstStart.IsZero() {
						continue
					}
				}

				// Calculate delta and adjust timestamps
				delta := targetStart.Sub(firstStart)
				start := span.StartTimestamp().AsTime().Add(delta)
				end := span.EndTimestamp().AsTime().Add(delta)

				span.SetStartTimestamp(pcommon.NewTimestampFromTime(start))
				span.SetEndTimestamp(pcommon.NewTimestampFromTime(end))
			}
		}
	}
}

// extractTraceID extracts the first trace ID from ptrace.Traces for retrieval testing.
func extractTraceID(t *testing.T, traces ptrace.Traces) pcommon.TraceID {
	require.Positive(t, traces.ResourceSpans().Len(), "Trace must have resource spans")
	rs := traces.ResourceSpans().At(0)
	require.Positive(t, rs.ScopeSpans().Len(), "Resource must have scope spans")
	ss := rs.ScopeSpans().At(0)
	require.Positive(t, ss.Spans().Len(), "Scope must have spans")
	return ss.Spans().At(0).TraceID()
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
