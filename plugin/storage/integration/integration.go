// Copyright (c) 2019 The Jaeger Authors.
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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

const (
	iterations = 30
)

// StorageIntegration holds components for storage integration test
type StorageIntegration struct {
	SpanWriter       spanstore.Writer
	SpanReader       spanstore.Reader
	DependencyWriter dependencystore.Writer
	DependencyReader dependencystore.Reader
	Fixtures         []*QueryFixtures
	// TODO: remove this flag after all storage plugins returns spanKind with operationNames
	NotSupportSpanKindWithOperation bool
	FixturesPath                    string

	// CleanUp() should ensure that the storage backend is clean before another test.
	// called either before or after each test, and should be idempotent
	CleanUp func() error

	// Refresh() should ensure that the storage backend is up to date before being queried.
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

func (s *StorageIntegration) waitForCondition(t *testing.T, predicate func(t *testing.T) bool) bool {
	for i := 0; i < iterations; i++ {
		t.Logf("Waiting for storage backend to update documents, iteration %d out of %d", i+1, iterations)
		if predicate(t) {
			return true
		}
		time.Sleep(100 * time.Millisecond) // Will wait up to 3 seconds at worst.
	}
	return predicate(t)
}

func (s *StorageIntegration) testGetServices(t *testing.T) {
	defer s.cleanUp(t)

	expected := []string{"example-service-1", "example-service-2", "example-service-3"}
	s.loadParseAndWriteExampleTrace(t)
	s.refresh(t)

	var actual []string
	found := s.waitForCondition(t, func(t *testing.T) bool {
		actual, err := s.SpanReader.GetServices(context.Background())
		require.NoError(t, err)
		return assert.ObjectsAreEqualValues(expected, actual)
	})

	if !assert.True(t, found) {
		t.Log("\t Expected:", expected)
		t.Log("\t Actual  :", actual)
	}
}

func (s *StorageIntegration) testGetLargeSpan(t *testing.T) {
	defer s.cleanUp(t)

	t.Log("Testing Large Trace over 10K ...")
	expected := s.loadParseAndWriteLargeTrace(t)
	expectedTraceID := expected.Spans[0].TraceID
	s.refresh(t)

	var actual *model.Trace
	found := s.waitForCondition(t, func(t *testing.T) bool {
		var err error
		actual, err = s.SpanReader.GetTrace(context.Background(), expectedTraceID)
		return err == nil && len(actual.Spans) == len(expected.Spans)
	})
	if !assert.True(t, found) {
		CompareTraces(t, expected, actual)
	}
}

func (s *StorageIntegration) testGetOperations(t *testing.T) {
	defer s.cleanUp(t)

	var expected []spanstore.Operation
	if s.NotSupportSpanKindWithOperation {
		expected = []spanstore.Operation{
			{Name: "example-operation-1"},
			{Name: "example-operation-3"},
			{Name: "example-operation-4"},
		}
	} else {
		expected = []spanstore.Operation{
			{Name: "example-operation-1"},
			{Name: "example-operation-3", SpanKind: "server"},
			{Name: "example-operation-4", SpanKind: "client"},
		}
	}
	s.loadParseAndWriteExampleTrace(t)
	s.refresh(t)

	var actual []spanstore.Operation
	found := s.waitForCondition(t, func(t *testing.T) bool {
		var err error
		actual, err = s.SpanReader.GetOperations(context.Background(),
			spanstore.OperationQueryParameters{ServiceName: "example-service-1"})
		require.NoError(t, err)
		return assert.ObjectsAreEqualValues(expected, actual)
	})

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
	found := s.waitForCondition(t, func(t *testing.T) bool {
		var err error
		actual, err = s.SpanReader.GetTrace(context.Background(), expectedTraceID)
		if err != nil {
			t.Log(err)
		}
		return err == nil && len(actual.Spans) == len(expected.Spans)
	})
	if !assert.True(t, found) {
		CompareTraces(t, expected, actual)
	}

	t.Run("NotFound error", func(t *testing.T) {
		fakeTraceID := model.TraceID{High: 0, Low: 0}
		trace, err := s.SpanReader.GetTrace(context.Background(), fakeTraceID)
		assert.Equal(t, spanstore.ErrTraceNotFound, err)
		assert.Nil(t, trace)
	})
}

func (s *StorageIntegration) testFindTraces(t *testing.T) {
	defer s.cleanUp(t)

	fixturesPath := s.FixturesPath
	if s.FixturesPath == "" {
		fixturesPath = "."
	}
	// Note: all cases include ServiceName + StartTime range
	s.Fixtures = append(s.Fixtures, LoadAndParseQueryTestCases(t, fmt.Sprintf("%s/fixtures/queries.json", fixturesPath))...)

	// Each query test case only specifies matching traces, but does not provide counterexamples.
	// To improve coverage we get all possible traces and store all of them before running queries.
	allTraceFixtures := make(map[string]*model.Trace)
	expectedTracesPerTestCase := make([][]*model.Trace, 0, len(s.Fixtures))
	for _, queryTestCase := range s.Fixtures {
		var expected []*model.Trace
		for _, traceFixture := range queryTestCase.ExpectedFixtures {
			trace, ok := allTraceFixtures[traceFixture]
			if !ok {
				trace = s.getTraceFixture(t, traceFixture)
				err := s.writeTrace(t, trace)
				require.NoError(t, err, "Unexpected error when writing trace %s to storage", traceFixture)
				allTraceFixtures[traceFixture] = trace
			}
			expected = append(expected, trace)
		}
		expectedTracesPerTestCase = append(expectedTracesPerTestCase, expected)
	}
	s.refresh(t)
	for i, queryTestCase := range s.Fixtures {
		t.Run(queryTestCase.Caption, func(t *testing.T) {
			expected := expectedTracesPerTestCase[i]
			actual := s.findTracesByQuery(t, queryTestCase.Query, expected)
			CompareSliceOfTraces(t, expected, actual)
		})
	}
}

func (s *StorageIntegration) findTracesByQuery(t *testing.T, query *spanstore.TraceQueryParameters, expected []*model.Trace) []*model.Trace {
	var traces []*model.Trace
	found := s.waitForCondition(t, func(t *testing.T) bool {
		var err error
		traces, err = s.SpanReader.FindTraces(context.Background(), query)
		if err == nil && tracesMatch(t, traces, expected) {
			return true
		}
		t.Logf("FindTraces: expected: %d, actual: %d, match: false", len(expected), len(traces))
		return false
	})
	require.True(t, found)
	return traces
}

func (s *StorageIntegration) writeTrace(t *testing.T, trace *model.Trace) error {
	for _, span := range trace.Spans {
		if err := s.SpanWriter.WriteSpan(context.Background(), span); err != nil {
			return err
		}
	}
	return nil
}

func (s *StorageIntegration) loadParseAndWriteExampleTrace(t *testing.T) *model.Trace {
	trace := s.getTraceFixture(t, "example_trace")
	err := s.writeTrace(t, trace)
	require.NoError(t, err, "Not expecting error when writing example_trace to storage")
	return trace
}

func (s *StorageIntegration) loadParseAndWriteLargeTrace(t *testing.T) *model.Trace {
	trace := s.getTraceFixture(t, "example_trace")
	span := trace.Spans[0]
	spns := make([]*model.Span, 1, 10008)
	trace.Spans = spns
	trace.Spans[0] = span
	for i := 1; i < 10008; i++ {
		s := new(model.Span)
		*s = *span
		s.StartTime = s.StartTime.Add(time.Second * time.Duration(i+1))
		trace.Spans = append(trace.Spans, s)
	}
	err := s.writeTrace(t, trace)
	require.NoError(t, err, "Not expecting error when writing example_trace to storage")
	return trace
}

func (s *StorageIntegration) getTraceFixture(t *testing.T, fixture string) *model.Trace {
	fixturesPath := s.FixturesPath
	if s.FixturesPath == "" {
		fixturesPath = "."
	}
	fileName := fmt.Sprintf("%s/fixtures/traces/%s.json", fixturesPath, fixture)
	return getTraceFixtureExact(t, fileName)
}

func getTraceFixtureExact(t *testing.T, fileName string) *model.Trace {
	var trace model.Trace
	loadAndParseJSONPB(t, fileName, &trace)
	return &trace
}

func loadAndParseJSONPB(t *testing.T, path string, object proto.Message) {
	// #nosec
	inStr, err := ioutil.ReadFile(path)
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

func loadAndParseJSON(t *testing.T, path string, object interface{}) {
	// #nosec
	inStr, err := ioutil.ReadFile(path)
	require.NoError(t, err, "Not expecting error when loading fixture %s", path)
	err = json.Unmarshal(correctTime(inStr), object)
	require.NoError(t, err, "Not expecting error when unmarshaling fixture %s", path)
}

// required, because we want to only query on recent traces, so we replace all the dates with recent dates.
func correctTime(json []byte) []byte {
	jsonString := string(json)
	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
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
	actual, err := s.DependencyReader.GetDependencies(context.Background(), time.Now(), 5*time.Minute)
	assert.NoError(t, err)
	assert.EqualValues(t, expected, actual)
}

// IntegrationTestAll runs all integration tests
func (s *StorageIntegration) IntegrationTestAll(t *testing.T) {
	t.Run("GetServices", s.testGetServices)
	t.Run("GetOperations", s.testGetOperations)
	t.Run("GetTrace", s.testGetTrace)
	t.Run("GetLargeSpans", s.testGetLargeSpan)
	t.Run("FindTraces", s.testFindTraces)
	t.Run("GetDependencies", s.testGetDependencies)
}
