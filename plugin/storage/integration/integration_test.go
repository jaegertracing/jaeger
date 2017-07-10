// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package integration

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/storage/spanstore"
)

const (
	iterations            = 30
	waitForBackendComment = "Waiting for storage backend to update documents, iteration %d out of %d"
)

type StorageIntegration struct {
	logger *zap.Logger
	writer spanstore.Writer
	reader spanstore.Reader

	// cleanUp() should ensure that the storage backend is clean before another test.
	// called either before or after each test, and should be idempotent
	cleanUp func() error

	// refresh() should ensure that the storage backend is up to date before being queried.
	// called between set-up and queries in each test
	refresh func() error
}

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

func (s *StorageIntegration) IntegrationTestGetServices(t *testing.T) {
	t.Log("Testing GetServices ...")

	expected := []string{"example-service-1", "example-service-2", "example-service-3"}
	s.getBasicFixture(t)
	require.NoError(t, s.refresh())

	var found bool

	var actual []string
	for i := 0; i < iterations; i++ {
		s.logger.Info(fmt.Sprintf(waitForBackendComment, i+1, iterations))
		actual, err := s.reader.GetServices()
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
	assert.NoError(t, s.cleanUp())
}

func (s *StorageIntegration) IntegrationTestGetOperations(t *testing.T) {
	t.Log("Testing GetOperations ...")

	expected := []string{"example-operation-1", "example-operation-3", "example-operation-4"}
	s.getBasicFixture(t)
	require.NoError(t, s.refresh())

	var found bool
	var actual []string
	for i := 0; i < iterations; i++ {
		s.logger.Info(fmt.Sprintf(waitForBackendComment, i+1, iterations))
		actual, err := s.reader.GetOperations("example-service-1")
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
	assert.NoError(t, s.cleanUp())
}

func (s *StorageIntegration) IntegrationTestGetTrace(t *testing.T) {
	t.Log("Testing GetTrace ...")

	expected := s.getBasicFixture(t)
	expectedTraceID := expected.Spans[0].TraceID
	require.NoError(t, s.refresh())

	var actual *model.Trace
	for i := 0; i < iterations; i++ {
		s.logger.Info(fmt.Sprintf(waitForBackendComment, i+1, iterations))
		var err error
		actual, err = s.reader.GetTrace(expectedTraceID)
		if err == nil && len(actual.Spans) == len(expected.Spans) {
			break
		}
		time.Sleep(100 * time.Millisecond) // Will wait up to 3 seconds at worst.
	}

	CompareTraces(t, expected, actual)
	assert.NoError(t, s.cleanUp())
}

func (s *StorageIntegration) IntegrationTestFindTraces(t *testing.T) {
	t.Log("Testing FindTraces ...")
	t.Log("\t(Note: all cases include ServiceName + StartTime range)")
	queries, err := getQueries()
	require.NoError(t, err)
	for _, query := range queries {
		t.Logf("\t\t* Query case: + %s", query.Caption)
		s.integrationTestFindTracesByQuery(t, query.Query, query.ExpectedFixtures)
	}
}

func (s *StorageIntegration) integrationTestFindTracesByQuery(t *testing.T, query *spanstore.TraceQueryParameters, expectedFixtures []string) {
	expected, err := getTraceFixtures(expectedFixtures)
	require.NoError(t, err)
	require.NoError(t, s.writeTraces(expected))
	require.NoError(t, s.refresh())

	var found bool
	var actual []*model.Trace
	for i := 0; i < iterations; i++ {
		s.logger.Info(fmt.Sprintf(waitForBackendComment, i+1, iterations))
		actual, err = s.reader.FindTraces(query)
		if err == nil {
			if found = tracesMatch(actual, expected); found {
				CompareSliceOfTraces(t, expected, actual)
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !assert.True(t, found) {
		CompareSliceOfTraces(t, expected, actual)
	}
	assert.NoError(t, s.cleanUp())
}

func (s *StorageIntegration) writeTraces(traces []*model.Trace) error {
	for _, trace := range traces {
		if err := s.writeTrace(trace); err != nil {
			return err
		}
	}
	return nil
}

func (s *StorageIntegration) writeTrace(trace *model.Trace) error {
	for _, span := range trace.Spans {
		if err := s.writer.WriteSpan(span); err != nil {
			return err
		}
	}
	return nil
}

func (s *StorageIntegration) getBasicFixture(t *testing.T) *model.Trace {
	trace, err := getTraceFixture("example_trace")
	require.NoError(t, err)
	require.NoError(t, s.writeTrace(trace))
	return trace
}

func getTraceFixture(fixture string) (*model.Trace, error) {
	var trace model.Trace
	fileName := fmt.Sprintf("fixtures/traces/%s.json", fixture)
	if err := getFixture(fileName, &trace); err != nil {
		return nil, err
	}
	return &trace, nil
}

func getTraceFixtures(fixtures []string) ([]*model.Trace, error) {
	traces := make([]*model.Trace, len(fixtures))
	for i, fixture := range fixtures {
		trace, err := getTraceFixture(fixture)
		if err != nil {
			return nil, err
		}
		traces[i] = trace
	}
	return traces, nil
}

func getQueries() ([]*QueryFixtures, error) {
	var queries []*QueryFixtures
	if err := getFixture("fixtures/queries.json", &queries); err != nil {
		return nil, err
	}
	return queries, nil
}

func getFixture(path string, object interface{}) error {
	inStr, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(correctTime(inStr), object)
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

func tracesMatch(actual []*model.Trace, expected []*model.Trace) bool {
	if len(actual) != len(expected) {
		return false
	}
	return spanCount(actual) == spanCount(expected)
}

func spanCount(traces []*model.Trace) int {
	var count int
	for _, trace := range traces {
		count += len(trace.Spans)
	}
	return count
}

func (s *StorageIntegration) IntegrationTestAll(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	s.IntegrationTestGetServices(t)
	s.IntegrationTestGetOperations(t)
	s.IntegrationTestGetTrace(t)
	s.IntegrationTestFindTraces(t)
}
