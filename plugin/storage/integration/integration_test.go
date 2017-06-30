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
	"context"
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

type StorageIntegration struct {
	ctx     context.Context
	logger  *zap.Logger
	writer  spanstore.Writer
	reader  spanstore.Reader
	cleanUp func() error
	refresh func() error // function called between set-up and queries in each test
}

type QueryFixtures struct {
	Caption        string
	Query          *spanstore.TraceQueryParameters
	ExpectedTraces []int
}

const (
	numOfTraceFixtures = 5
	timeOut            = 3 // in seconds
)

func (s *StorageIntegration) IntegrationTestGetServices(t *testing.T) {
	t.Log("Testing GetServices ...")
	expected := []string{"query01-service", "query06-service", "query07-service", "query12-service", "query13-service"}
	trace, err := getTraceFixture(1)
	require.NoError(t, err)
	require.NoError(t, s.writeTrace(trace))

	s.refresh()

	var found bool
	iterations := 10 * timeOut
	var actual []string
	for i := 0; i < iterations; i++ {
		s.logger.Info(fmt.Sprintf("Waiting for ES to update documents, iteration %d out of %d", i+1, iterations))
		actual, err = s.reader.GetServices()
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

	expected := []string{"operation-list-test1", "operation-list-test2", "operation-list-test3"}
	traces, err := getTraceFixtures()
	require.NoError(t, err)
	require.NoError(t, s.writeTraces(traces))

	s.refresh()

	var found bool
	iterations := 10 * timeOut
	var actual []string
	for i := 0; i < iterations; i++ {
		s.logger.Info(fmt.Sprintf("Waiting for ES to update documents, iteration %d out of %d", i+1, iterations))
		actual, err = s.reader.GetOperations("query05-service")
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

	expected, err := getTraceFixture(1)
	require.NoError(t, err)
	require.NoError(t, s.writeTrace(expected))
	traceID := model.TraceID{Low: 1}

	s.refresh()

	var found bool
	iterations := 10 * timeOut
	var actual *model.Trace
	for i := 0; i < iterations; i++ {
		s.logger.Info(fmt.Sprintf("Waiting for ES to update documents, iteration %d out of %d", i+1, iterations))
		actual, err = s.reader.GetTrace(traceID)
		if found = err == nil && len(actual.Spans) == len(expected.Spans); found {
			CompareModelTraces(t, expected, actual)
			break
		}
		time.Sleep(100 * time.Millisecond) // Will wait up to 3 seconds at worst.
	}

	if !assert.True(t, found) {
		t.Log("\t Expected:", expected)
		t.Log("\t Actual  :", actual)
		for _, span := range actual.Spans {
			t.Log(span.TraceID, span.Process.ServiceName)
		}
	}
	assert.NoError(t, s.cleanUp())
}

func (s *StorageIntegration) IntegrationTestFindTraces(t *testing.T) {
	t.Log("Testing FindTraces ...")
	t.Log("\t(Note: all cases include ServiceName + StartTime range)")
	queries, err := getQueries()
	require.NoError(t, err)
	for _, query := range queries {
		t.Logf("\t\t* Query case: + %s", query.Caption)
		s.integrationTestFindTracesByQuery(t, query.Query, query.ExpectedTraces)
	}
}

func (s *StorageIntegration) integrationTestFindTracesByQuery(t *testing.T, query *spanstore.TraceQueryParameters, expectedTraces []int) {
	traces, err := getTraceFixtures()
	require.NoError(t, err)
	expected := getSubsetOfTraces(traces, expectedTraces)
	require.NoError(t, s.writeTraces(expected))
	require.NoError(t, s.refresh())

	iterations := 10 * timeOut
	var found bool
	for i := 0; i < iterations; i++ {
		s.logger.Info(fmt.Sprintf("Waiting for ES to update documents, iteration %d out of %d", i+1, iterations))
		actual, err := s.reader.FindTraces(query)
		require.NoError(t, err)

		if len(actual) == query.NumTraces {
			found = true
			break
		}
		if found = err == nil && tracesMatch(actual, expected); found {
			CompareListOfModelTraces(t, expected, actual)
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	assert.True(t, found)
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

func getSubsetOfTraces(traces []*model.Trace, expectedTraces []int) []*model.Trace {
	retTraces := make([]*model.Trace, len(expectedTraces))
	for i, traceNum := range expectedTraces {
		retTraces[i] = traces[traceNum-1]
	}
	return retTraces
}

func getTraceFixtures() ([]*model.Trace, error) {
	traces := make([]*model.Trace, numOfTraceFixtures)
	for i := 0; i < numOfTraceFixtures; i++ {
		trace, err := getTraceFixture(i + 1)
		if err != nil {
			return nil, err
		}
		traces[i] = trace
	}
	return traces, nil
}

func getTraceFixture(i int) (*model.Trace, error) {
	var trace model.Trace
	fileName := fmt.Sprintf("fixtures/traces/trace_%02d.json", i)
	err := getFixture(fileName, &trace)
	if err != nil {
		return nil, err
	}
	return &trace, nil
}

func getQueries() ([]*QueryFixtures, error) {
	var queries []*QueryFixtures
	err := getFixture("fixtures/queries.json", &queries)
	if err != nil {
		return nil, err
	}
	return queries, nil
}

func getFixture(path string, object interface{}) error {
	inStr, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	err = json.Unmarshal(normalizeTime(inStr), object)
	return err
}

func normalizeTime(json []byte) []byte {
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
	return countOfSpans(actual) == countOfSpans(expected)
}

func countOfSpans(traces []*model.Trace) int {
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
