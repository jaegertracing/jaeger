// Copyright (c) 2017 Uber Technologies, Inc.
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

package integration

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	iterations            = 30
	waitForBackendComment = "Waiting for storage backend to update documents, iteration %d out of %d"
)

type StorageIntegration struct {
	// TODO make these public
	logger           *zap.Logger
	SpanWriter       spanstore.Writer
	SpanReader       spanstore.Reader
	DependencyWriter dependencystore.Writer
	DependencyReader dependencystore.Reader

	// cleanUp() should ensure that the storage backend is clean before another test.
	// called either before or after each test, and should be idempotent
	CleanUp func() error

	// refresh() should ensure that the storage backend is up to date before being queried.
	// called between set-up and queries in each test
	Refresh func() error
}

// === SpanStore Integration Tests ===

// QueryFixtures and TraceFixtures are under ./fixtures/queries.json and ./fixtures/traces/*.json respectively.
// Each query fixture includes:
// 	Caption: describes the query we are testing
// 	Query: the query we are testing
//	ExpectedFixture: the trace fixture that we want back from these queries.
// Queries are not necessarily numbered, but since each query requires a service name,
// the service name is formatted "query##-service".
type QueryFixtures struct {
	Caption          string
	Query            *spanstore.TraceQueryParameters
	ExpectedFixtures []string
}

func (s *StorageIntegration) cleanUp(t *testing.T) {
	require.NotNil(t, s.CleanUp, "CleanUp function must be provided")
	require.NoError(t, s.CleanUp())
}

func (s *StorageIntegration) refresh(t *testing.T) {
	require.NotNil(t, s.Refresh, "Refresh function must be provided")
	require.NoError(t, s.Refresh())
}

func (s *StorageIntegration) testGetServices(t *testing.T) {
	defer s.cleanUp(t)

	expected := []string{"example-service-1", "example-service-2", "example-service-3"}
	s.loadParseAndWriteExampleTrace(t)
	s.refresh(t)

	var found bool

	var actual []string
	for i := 0; i < iterations; i++ {
		s.logger.Info(fmt.Sprintf(waitForBackendComment, i+1, iterations))
		actual, err := s.SpanReader.GetServices()
		require.NoError(t, err)
		if found = assert.ObjectsAreEqualValues(expected, actual); found {
			break
		}
		time.Sleep(100 * time.Millisecond) // Will wait up to 3 seconds at worst.
	}

	if !assert.True(t, found) {
		t.Log("\t Expected:", expected)
		t.Log("\t Actual  :", actual)
	}
}

func (s *StorageIntegration) testGetOperations(t *testing.T) {
	defer s.cleanUp(t)

	expected := []string{"example-operation-1", "example-operation-3", "example-operation-4"}
	s.loadParseAndWriteExampleTrace(t)
	s.refresh(t)

	var found bool
	var actual []string
	for i := 0; i < iterations; i++ {
		s.logger.Info(fmt.Sprintf(waitForBackendComment, i+1, iterations))
		actual, err := s.SpanReader.GetOperations("example-service-1")
		require.NoError(t, err)
		if found = assert.ObjectsAreEqualValues(expected, actual); found {
			break
		}
		time.Sleep(100 * time.Millisecond) // Will wait up to 3 seconds at worst.
	}

	if !assert.True(t, found) {
		t.Log("\t Expected:", expected)
		t.Log("\t Actual  :", actual)
	}
}

func (s *StorageIntegration) testGetTrace(t *testing.T) {
	defer s.cleanUp(t)

	expected := s.loadParseAndWriteExampleTrace(t)
	expectedTraceID := expected.Spans[0].TraceID
	s.refresh(t)

	var actual *model.Trace
	for i := 0; i < iterations; i++ {
		s.logger.Info(fmt.Sprintf(waitForBackendComment, i+1, iterations))
		var err error
		actual, err = s.SpanReader.GetTrace(expectedTraceID)
		if err == nil && len(actual.Spans) == len(expected.Spans) {
			break
		}
		time.Sleep(100 * time.Millisecond) // Will wait up to 3 seconds at worst.
	}

	CompareTraces(t, expected, actual)
}

func (s *StorageIntegration) testFindTraces(t *testing.T) {
	defer s.CleanUp()

	// Note: all cases include ServiceName + StartTime range
	queryTestCases := loadAndParseQueryTestCases(t)

	// Each query test case only specifies matching traces, but does not provide counterexamples.
	// To improve coverage we get all possible traces and store all of them before running queries.
	allTraceFixtures := make(map[string]*model.Trace)
	expectedTracesPerTestCase := make([][]*model.Trace, 0, len(queryTestCases))
	for _, queryTestCase := range queryTestCases {
		var expected []*model.Trace
		for _, traceFixture := range queryTestCase.ExpectedFixtures {
			trace, ok := allTraceFixtures[traceFixture]
			if !ok {
				trace = getTraceFixture(t, traceFixture)
				err := s.writeTrace(t, trace)
				require.NoError(t, err, "Unexpected error when writing trace %s to storage", traceFixture)
				allTraceFixtures[traceFixture] = trace
			}
			expected = append(expected, trace)
		}
		expectedTracesPerTestCase = append(expectedTracesPerTestCase, expected)
	}
	s.refresh(t)
	for i, queryTestCase := range queryTestCases {
		t.Run(queryTestCase.Caption, func(t *testing.T) {
			expected := expectedTracesPerTestCase[i]
			actual := s.findTracesByQuery(t, queryTestCase.Query, expected)
			CompareSliceOfTraces(t, expected, actual)
		})
	}
}

func (s *StorageIntegration) findTracesByQuery(t *testing.T, query *spanstore.TraceQueryParameters, expected []*model.Trace) []*model.Trace {
	for i := 0; i < iterations; i++ {
		s.logger.Info(fmt.Sprintf(waitForBackendComment, i+1, iterations))
		traces, err := s.SpanReader.FindTraces(query)
		if err == nil && tracesMatch(t, traces, expected) {
			return traces
		}
		t.Logf("FindTraces: expected: %d, actual: %d, match: false", len(expected), len(traces))
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("Failed to find expected traces")
	return nil
}

func (s *StorageIntegration) writeTraces(t *testing.T, traces []*model.Trace) error {
	for _, trace := range traces {
		if err := s.writeTrace(t, trace); err != nil {
			return err
		}
	}
	return nil
}

func (s *StorageIntegration) writeTrace(t *testing.T, trace *model.Trace) error {
	for _, span := range trace.Spans {
		if err := s.SpanWriter.WriteSpan(span); err != nil {
			return err
		}
	}
	return nil
}

func (s *StorageIntegration) loadParseAndWriteExampleTrace(t *testing.T) *model.Trace {
	trace := getTraceFixture(t, "example_trace")
	err := s.writeTrace(t, trace)
	require.NoError(t, err, "Not expecting error when writing example_trace to storage")
	return trace
}

func getTraceFixture(t *testing.T, fixture string) *model.Trace {
	var trace model.Trace
	fileName := fmt.Sprintf("fixtures/traces/%s.json", fixture)
	loadAndParseFixture(t, fileName, &trace)
	return &trace
}

func getTraceFixtures(t *testing.T, fixtures []string) []*model.Trace {
	traces := make([]*model.Trace, len(fixtures))
	for i, fixture := range fixtures {
		trace := getTraceFixture(t, fixture)
		traces[i] = trace
	}
	return traces
}

func loadAndParseQueryTestCases(t *testing.T) []*QueryFixtures {
	var queries []*QueryFixtures
	loadAndParseFixture(t, "fixtures/queries.json", &queries)
	return queries
}

func loadAndParseFixture(t *testing.T, path string, object interface{}) {
	inStr, err := ioutil.ReadFile(path)
	require.NoError(t, err, "Not expecting error when loading fixture %s", path)
	err = json.Unmarshal(correctTime(inStr), object)
	require.NoError(t, err, "Not expecting error when unmarshaling fixture %s", path)
}

// required, because we want to only query on recent traces, so we replace all the dates with recent dates.
func correctTime(json []byte) []byte {
	jsonString := string(json)
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	retString := strings.Replace(jsonString, "2017-01-26", today, -1)
	retString = strings.Replace(retString, "2017-01-25", yesterday, -1)
	return []byte(retString)
}

func tracesMatch(t *testing.T, actual []*model.Trace, expected []*model.Trace) bool {
	if !assert.Equal(t, len(expected), len(actual), "Expecting certain number of traces") {
		return false
	}
	return assert.Equal(t, spanCount(expected), spanCount(actual), "Expecting certain number of spans")
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

	defer s.cleanUp(t)

	expected := []model.DependencyLink{
		{
			Parent:    "hello",
			Child:     "world",
			CallCount: uint64(1),
		},
		{
			Parent:    "world",
			Child:     "hello",
			CallCount: uint64(3),
		},
	}
	require.NoError(t, s.DependencyWriter.WriteDependencies(time.Now(), expected))
	s.refresh(t)
	actual, err := s.DependencyReader.GetDependencies(time.Now(), 5*time.Minute)
	assert.NoError(t, err)
	assert.EqualValues(t, expected, actual)
}

func (s *StorageIntegration) IntegrationTestAll(t *testing.T) {
	t.Run("GetServices", s.testGetServices)
	t.Run("GetOperations", s.testGetOperations)
	t.Run("GetTrace", s.testGetTrace)
	t.Run("FindTraces", s.testFindTraces)
	t.Run("GetDependencies", s.testGetDependencies)
}
