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
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	samplemodel "github.com/jaegertracing/jaeger/cmd/collector/app/sampling/model"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/samplingstore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/jaegertracing/jaeger/storage_v2/tracestore"
	otlp2jaeger "github.com/open-telemetry/opentelemetry-collector-contrib/pkg/translator/jaeger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
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
	SpanWriter        spanstore.Writer
	SpanReader        spanstore.Reader
	ArchiveSpanReader spanstore.Reader
	ArchiveSpanWriter spanstore.Writer
	DependencyWriter  dependencystore.Writer
	DependencyReader  dependencystore.Reader
	SamplingStore     samplingstore.Store
	Fixtures          []*QueryFixtures
	TraceReader       tracestore.Reader
	TraceWriter       tracestore.Writer

	// TODO: remove this after all storage backends return spanKind from GetOperations
	GetOperationsMissingSpanKind bool

	// TODO: remove this after all storage backends return Source column from GetDependencies

	GetDependenciesReturnsSource bool

	// Skip Archive Test if not supported by the storage backend
	SkipArchiveTest bool

	// List of tests which has to be skipped, it can be regex too.
	SkipList []string

	// CleanUp() should ensure that the storage backend is clean before another test.
	// called either before or after each test, and should be idempotent
	CleanUp func(t *testing.T)
}

// === SpanStore Integration Tests ===

// QueryFixtures and TraceFixtures are under ./fixtures/queries.json and ./fixtures/traces/*.json respectively.
// Each query fixture includes:
// - Caption: describes the query we are testing
// - Query: the query we are testing
// - ExpectedFixture: the trace fixture that we want back from these queries.
// Queries are not necessarily numbered, but since each query requires a service name,
// the service name is formatted "query##-service".
type QueryFixtures struct {
	Caption          string
	Query            *tracestore.TraceQueryParams
	ExpectedFixtures []string
}

func (s *StorageIntegration) cleanUp(t *testing.T) {
	require.NotNil(t, s.CleanUp, "CleanUp function must be provided")
	s.CleanUp(t)
}

func SkipUnlessEnv(t *testing.T, storage ...string) {
	env := os.Getenv("STORAGE")
	for _, s := range storage {
		if env == s {
			return
		}
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
			t.Log("🛑 Found unexpected services!")
			for _, service := range actual {
				traceSeq := s.TraceReader.FindTraces(context.Background(), tracestore.TraceQueryParams{
					ServiceName: service,
				})

				hasError := false
				traceSeq(func(traces []ptrace.Traces, err error) bool {
					if err != nil {
						t.Log(err)
						hasError = true
						return false
					}
					for _, otelTrace := range traces {
						t.Logf("Retrieved trace for service '%s': %v", service, otelTrace)
					}
					return true
				})

				if hasError {
					t.Logf("Error processing traces for service '%s'", service)
					return false
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


func (s *StorageIntegration) testArchiveTrace(t *testing.T) {
	s.skipIfNeeded(t)
	if s.SkipArchiveTest {
		t.Skip("Skipping ArchiveTrace test because archive reader or writer is nil")
	}
	defer s.cleanUp(t)
	tID := model.NewTraceID(uint64(11), uint64(22))
	expected := &model.Span{
		OperationName: "archive_span",
		StartTime:     time.Now().Add(-time.Hour * 72 * 5).Truncate(time.Microsecond),
		TraceID:       tID,
		SpanID:        model.NewSpanID(55),
		References:    []model.SpanRef{},
		Process:       model.NewProcess("archived_service", model.KeyValues{}),
	}

	require.NoError(t, s.ArchiveSpanWriter.WriteSpan(context.Background(), expected))

	var actual *model.Trace
	found := s.waitForCondition(t, func(_ *testing.T) bool {
		var err error
		actual, err = s.ArchiveSpanReader.GetTrace(context.Background(), spanstore.GetTraceParameters{TraceID: tID})
		return err == nil && len(actual.Spans) == 1
	})
	require.True(t, found)
	CompareTraces(t, &model.Trace{Spans: []*model.Span{expected}}, actual)
}

func (s *StorageIntegration) testGetLargeSpan(t *testing.T) {
	s.skipIfNeeded(t)
	defer s.cleanUp(t)

	t.Log("Testing Large Trace over 10K with duplicate IDs...")

	expected := s.writeLargeTraceWithDuplicateSpanIds(t)
	expectedTraceID := expected.Spans[0].TraceID

	var actual *model.Trace
	found := s.waitForCondition(t, func(_ *testing.T) bool {
		var err error
		actual, err = s.SpanReader.GetTrace(context.Background(), spanstore.GetTraceParameters{TraceID: expectedTraceID})
		return err == nil && len(actual.Spans) >= len(expected.Spans)
	})

	if !assert.True(t, found) {
		CompareTraces(t, expected, actual)
		return
	}

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

func (s *StorageIntegration) testGetOperations(t *testing.T) {
	s.skipIfNeeded(t)
	defer s.cleanUp(t)

	var expected []spanstore.Operation
	if s.GetOperationsMissingSpanKind {
		expected = []spanstore.Operation{
			{Name: "example-operation-1"},
			{Name: "example-operation-3"},
			{Name: "example-operation-4"},
		}
	} else {
		expected = []spanstore.Operation{
			{Name: "example-operation-1", SpanKind: "unspecified"},
			{Name: "example-operation-3", SpanKind: "server"},
			{Name: "example-operation-4", SpanKind: "client"},
		}
	}
	s.loadParseAndWriteExampleTrace(t)

	var actual []spanstore.Operation
	found := s.waitForCondition(t, func(t *testing.T) bool {
		var err error
		actual, err = s.SpanReader.GetOperations(context.Background(),
			spanstore.OperationQueryParameters{ServiceName: "example-service-1"})
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

// func (s *StorageIntegration) testGetTrace(t *testing.T) {
// 	s.skipIfNeeded(t)
// 	defer s.cleanUp(t)

// 	expected := s.loadParseAndWriteExampleTrace(t)
// 	expectedTraceID := expected.Spans[0].TraceID

// 	var actual *model.Trace
// 	found := s.waitForCondition(t, func(t *testing.T) bool {
// 		var err error
// 		actual, err = s.SpanReader.GetTrace(context.Background(), spanstore.GetTraceParameters{TraceID: expectedTraceID})
// 		if err != nil {
// 			t.Log(err)
// 		}
// 		return err == nil && len(actual.Spans) == len(expected.Spans)
// 	})
// 	if !assert.True(t, found) {
// 		CompareTraces(t, expected, actual)
// 	}

// 	t.Run("NotFound error", func(t *testing.T) {
// 		fakeTraceID := model.TraceID{High: 0, Low: 1}
// 		trace, err := s.SpanReader.GetTrace(context.Background(), spanstore.GetTraceParameters{TraceID: fakeTraceID})
// 		assert.Equal(t, spanstore.ErrTraceNotFound, err)
// 		assert.Nil(t, trace)
// 	})
// }

func (s *StorageIntegration) testGetTrace(t *testing.T) {
	s.skipIfNeeded(t)
	defer s.cleanUp(t)

	expected := s.loadParseAndWriteExampleTrace(t)
	expectedTraceID := expected.Spans[0].TraceID

	var actual *model.Trace
	found := s.waitForCondition(t, func(t *testing.T) bool {
		var err error
		actual, err = s.SpanReader.GetTrace(context.Background(), spanstore.GetTraceParameters{TraceID: expectedTraceID})
		if err != nil {
			t.Log(err)
		}
		return err == nil && len(actual.Spans) == len(expected.Spans)
	})
	if !assert.True(t, found) {
		CompareTraces(t, expected, actual)
	}

	t.Run("NotFound error", func(t *testing.T) {
		fakeTraceID := model.TraceID{High: 0, Low: 1}
		trace, err := s.SpanReader.GetTrace(context.Background(), spanstore.GetTraceParameters{TraceID: fakeTraceID})
		assert.Equal(t, spanstore.ErrTraceNotFound, err)
		assert.Nil(t, trace)
	})
}

func (s *StorageIntegration) testFindTraces(t *testing.T) {
	s.skipIfNeeded(t)
	defer s.cleanUp(t)

	// Load query test cases
	s.Fixtures = append(s.Fixtures, LoadAndParseQueryTestCases(t, "fixtures/queries.json")...)

	// Prepare a map to store all trace fixtures and a slice for expected traces per test case
	allTraceFixtures := make(map[string]ptrace.Traces)
	expectedTracesPerTestCase := make([][]ptrace.Traces, 0, len(s.Fixtures))

	// Process each query test case to prepare expected traces
	for _, queryTestCase := range s.Fixtures {
		var expected []ptrace.Traces
		for _, traceFixture := range queryTestCase.ExpectedFixtures {
			trace, ok := allTraceFixtures[traceFixture]
			if !ok {
				trace = s.getTraceFixture(t, traceFixture) // Load the trace fixture
				s.writeTrace(t, trace)                     // Write the trace into the storage
				allTraceFixtures[traceFixture] = trace     // Cache the trace fixture
			}
			expected = append(expected, trace)
		}
		expectedTracesPerTestCase = append(expectedTracesPerTestCase, expected)
	}

	// Run tests for each query test case
	for i, queryTestCase := range s.Fixtures {
		t.Run(queryTestCase.Caption, func(t *testing.T) {
			s.skipIfNeeded(t)
			expected := expectedTracesPerTestCase[i]
			actual := s.findTracesByQuery(t, queryTestCase.Query, expected)

			// Compare the expected and actual traces
			CompareSliceOfTraces(t, expected, actual)
		})
	}
}

// func (s *StorageIntegration) findTracesByQuery(t *testing.T, query *tracestore.TraceQueryParams, expected []*ptrace.Traces) []ptrace.Traces {
// 	var traces []*model.Trace
// 	found := s.waitForCondition(t, func(t *testing.T) bool {
// 		var err error
// 		traces, err = s.SpanReader.FindTraces(context.Background(), query)
// 		if err != nil {
// 			t.Log(err)
// 			return false
// 		}
// 		if len(expected) != len(traces) {
// 			t.Logf("Expecting certain number of traces: expected: %d, actual: %d", len(expected), len(traces))
// 			return false
// 		}
// 		if spanCount(expected) != spanCount(traces) {
// 			t.Logf("Excepting certain number of spans: expected: %d, actual: %d", spanCount(expected), spanCount(traces))
// 			return false
// 		}
// 		return true
// 	})
// 	require.True(t, found)
// 	return traces
// }
func (s *StorageIntegration) findTracesByQuery(t *testing.T, query tracestore.TraceQueryParams, expected []ptrace.Traces) []ptrace.Traces {
	var collected []ptrace.Traces
	var lastErr error
  
	found := s.waitForCondition(t, func(t *testing.T) bool {
	    collected = nil // Reset for each attempt
	    traceSeq := s.TraceReader.FindTraces(context.Background(), query)
	    
	    traceSeq(func(traces []ptrace.Traces, err error) bool {
		  if err != nil {
			lastErr = err
			t.Log(err)
			return false
		  }
		  collected = append(collected, traces...)
		  return true
	    })
  
	    if lastErr != nil {
		  return false
	    }
  
	    if len(expected) != len(collected) {
		  t.Logf("Expecting certain number of traces: expected %d, actual %d", len(expected), len(collected))
		  return false
	    }
  
	    // Compare span counts
	    expectedSpans := 0
	    actualSpans := 0
	    for _, trace := range expected {
		  expectedSpans += trace.SpanCount()
	    }
	    for _, trace := range collected {
		  actualSpans += trace.SpanCount()
	    }
	    
	    if expectedSpans != actualSpans {
		  t.Logf("Expecting certain number of spans: expected %d, actual %d", expectedSpans, actualSpans)
		  return false
	    }
  
	    return true
	})
  
	require.True(t, found)
	return collected
  }

// func (s *StorageIntegration) writeTrace(t *testing.T, trace *model.Trace) {
// 	t.Logf("%-23s Writing trace with %d spans", time.Now().Format("2006-01-02 15:04:05.999"), len(trace.Spans))
// 	for _, span := range trace.Spans {
// 		err := s.SpanWriter.WriteSpan(context.Background(), span)
// 		require.NoError(t, err, "Not expecting error when writing trace to storage")
// 	}
// }

func (s *StorageIntegration) writeTrace(t *testing.T, td ptrace.Traces) {
	t.Logf("%-23s Writing trace with %d spans", time.Now().Format("2006-01-02 15:04:05.999"), td.SpanCount())
	err := s.TraceWriter.WriteTraces(context.Background(), td)
	if err != nil {
		t.Log(err)
	}
}

func (s *StorageIntegration) loadParseAndWriteExampleTrace(t *testing.T) ptrace.Traces {
	trace := s.getTraceFixture(t, "example_trace")
	s.writeTrace(t, trace)
	return trace
}
func (s *StorageIntegration) writeLargeTraceWithDuplicateSpanIds(t *testing.T) ptrace.Traces {
	// Load the example trace fixture
	trace := s.getTraceFixture(t, "example_trace")

	// Get the first resource span to use as a template
	resourceSpans := trace.ResourceSpans()
	if resourceSpans.Len() == 0 {
		t.Fatalf("No ResourceSpans found in the trace fixture")
	}
	originalSpans := resourceSpans.At(0).ScopeSpans().At(0).Spans()
	if originalSpans.Len() == 0 {
		t.Fatalf("No spans found in the ResourceSpan")
	}

	// Prepare a new trace with a large number of spans
	newTrace := ptrace.NewTraces()
	newResourceSpans := newTrace.ResourceSpans().AppendEmpty()
	resourceSpans.At(0).CopyTo(newResourceSpans)

	newScopeSpans := newResourceSpans.ScopeSpans().AppendEmpty()
	originalSpans.At(0).CopyTo(newScopeSpans.Spans().AppendEmpty()) // Copy the first span as a template

	for i := 0; i < 10008; i++ {
		newSpan := newScopeSpans.Spans().AppendEmpty()
		originalSpans.At(0).CopyTo(newSpan)

		if i%100 == 0 {
			// Duplicate span ID every 100 spans
			newSpan.SetSpanID(originalSpans.At(0).SpanID())
		} else {
			// Set unique span ID
			newSpan.SetSpanID(pcommon.NewSpanIDEmpty())
		}

		// Adjust the start time to make each span unique
		newSpan.SetStartTimestamp(originalSpans.At(0).StartTimestamp() + pcommon.Timestamp(i*1e9))
	}

	// Write the new trace
	s.writeTrace(t, newTrace)
	return newTrace
}


func (*StorageIntegration) getTraceFixture(t *testing.T, fixture string) ptrace.Traces {
	fileName := fmt.Sprintf("fixtures/traces/%s.json", fixture)

	return getTraceFixtureExact(t, fileName)
}

// func getTraceFixtureExact(t *testing.T, fileName string) *model.Trace {
// 	var trace model.Trace
// 	loadAndParseJSONPB(t, fileName, &trace)

//		return &trace
//	}
func getTraceFixtureExact(t *testing.T, fileName string) ptrace.Traces {
	var modelTrace model.Trace
	loadAndParseJSONPB(t, fileName, &modelTrace)

	// Create batches for each process
	batch  := &model.Batch{
		Spans: modelTrace.Spans,
	}	
	traces , err := otlp2jaeger.ProtoToTraces([]*model.Batch{batch})
	if err != nil {
		t.Log("Failed to Convert Trace: %v", err )
	}
	return traces
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
		t.Skipf("Skipping GetDependencies test because dependency reader or writer is nil")
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

	require.NoError(t, s.DependencyWriter.WriteDependencies(time.Now(), expected))

	var actual []model.DependencyLink
	found := s.waitForCondition(t, func(t *testing.T) bool {
		var err error
		actual, err = s.DependencyReader.GetDependencies(context.Background(), time.Now(), 5*time.Minute)
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
	t.Run("ArchiveTrace", s.testArchiveTrace)
	t.Run("GetDependencies", s.testGetDependencies)
	t.Run("GetThroughput", s.testGetThroughput)
	t.Run("GetLatestProbability", s.testGetLatestProbability)
}

// RunTestSpanstore runs only span related integration tests
func (s *StorageIntegration) RunSpanStoreTests(t *testing.T) {
	t.Run("GetServices", s.testGetServices)
	t.Run("GetOperations", s.testGetOperations)
	t.Run("GetTrace", s.testGetTrace)
	t.Run("GetLargeSpans", s.testGetLargeSpan)
	t.Run("FindTraces", s.testFindTraces)
}
