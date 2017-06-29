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
	"fmt"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/storage/spanstore"
	"bytes"
	"io/ioutil"
	"encoding/json"
)

type StorageIntegration struct {
	ctx     context.Context
	logger  *zap.Logger
	writer  spanstore.Writer
	reader  spanstore.Reader
	cleanUp func() error
	refresh func() error // function called between set-up and queries in each test
}

type QueryToExpected struct {
	Caption string
	Query *spanstore.TraceQueryParameters
	ExpectedTraces []int
}

const (
	floatTagVal  = "95.0421"
	intTagVal    = "950421"
	stringTagVal = "xyz"
	boolTagVal   = "true"

	numOfQueries = 5
	numOfTraceFixtures = 5

	defaultNumSpans = 5
	timeOut         = 3 // in seconds
)

var (
	randomTags = map[string]string{
		"tag1": intTagVal,
		"tag2": floatTagVal,
		"tag3": stringTagVal,
		"tag4": boolTagVal,
		"tag5": intTagVal,
		"tag6": stringTagVal,
		"tag7": stringTagVal,
	}
	randomServices   = []string{"service1", "service2", "service3", "service4"}
	randomOperations = []string{"op1", "op2", "op3", "op4"}

	// To add more various queries, add more TraceQueryParameters in differentQueries.
	// This should be sufficient; no need for extra code below.
	// Be sure to make the query large enough to capture some traces.
	// The below initializeTraces fn will fail if a query cannot capture traces.
	differentQueries = []*spanstore.TraceQueryParameters{
		{
			ServiceName:  randomService(),
			StartTimeMin: time.Now().Add(-3 * time.Hour),
			StartTimeMax: time.Now(),
			NumTraces:    numOfTraceFixtures, //set to numOfInitTraces if you don't care about number of traces retrieved, and you want all
		},
		{
			ServiceName:   randomService(),
			OperationName: randomOperation(),
			StartTimeMin:  time.Now().Add(-3 * time.Hour),
			StartTimeMax:  time.Now(),
			NumTraces:     numOfTraceFixtures,
		},
		{
			ServiceName: randomService(),
			Tags: map[string]string{
				"tag1": intTagVal,
				"tag3": stringTagVal,
			},
			StartTimeMin: time.Now().Add(-3 * time.Hour),
			StartTimeMax: time.Now(),
			NumTraces:    numOfTraceFixtures,
		},
		{
			ServiceName:  randomService(),
			StartTimeMin: time.Now().Add(-3 * time.Hour),
			StartTimeMax: time.Now(),
			DurationMin:  100 * time.Millisecond,
			DurationMax:  400 * time.Millisecond,
			NumTraces:    numOfTraceFixtures,
		},
		{
			ServiceName:  randomService(),
			StartTimeMin: time.Now().Add(-3 * time.Hour),
			StartTimeMax: time.Now(),
			NumTraces:    2,
		},
	}
)

func randomTimeAndDuration() (time.Time, time.Duration) {
	randomStartTime := time.Now().Add(time.Duration(-1*rand.Intn(80000)) * time.Second).Round(time.Millisecond) // randomizing the startTime
	randomDuration := 200*time.Millisecond + time.Duration(rand.Intn(800))*time.Millisecond
	return randomStartTime, randomDuration
}

func randomTimeAndDurationBetween(startTime time.Time, duration time.Duration) (time.Time, time.Duration) {
	randomStartDuration := rand.Intn(int(duration)) / 1000 * 1000
	randomDuration := rand.Intn(int(duration) - randomStartDuration) / 1000 * 1000
	return startTime.Add(time.Duration(randomStartDuration)), time.Duration(randomDuration)
}

func randomOperation() string {
	return randomOperations[rand.Intn(len(randomOperations))]
}

func randomService() string {
	return randomServices[rand.Intn(len(randomServices))]
}

func someRandomTags() model.KeyValues {
	subsetOfTags := make(map[string]string)

	for key, val := range randomTags {
		if rand.Intn(3) == 0 {
			subsetOfTags[key] = val
		}
	}
	return getTagsFromMap(subsetOfTags)
}

func createRandomSpan(traceID model.TraceID, startTime time.Time, duration time.Duration) *model.Span {
	return createSpanWithParams(traceID, randomService(), randomOperation(), startTime, duration)
}

func createSpanWithParams(traceID model.TraceID, service string, operation string, startTime time.Time, duration time.Duration) *model.Span {
	randomTime, randomDuration := randomTimeAndDurationBetween(startTime, duration)

	return &model.Span{
		TraceID:       traceID,
		SpanID:        model.SpanID(uint64(rand.Uint32())),
		ParentSpanID:  model.SpanID(uint64(rand.Uint32())),
		OperationName: operation,
		StartTime:     randomTime,
		Duration:      randomDuration,
		Tags:          someRandomTags(),
		Process: &model.Process{
			ServiceName: service,
			Tags:        someRandomTags(),
		},
		Logs: []model.Log{
			{Fields: []model.KeyValue(someRandomTags()), Timestamp: randomTime},
			{Fields: []model.KeyValue(someRandomTags()), Timestamp: randomTime},
		},
		References: []model.SpanRef{},
	}
}

func (s *StorageIntegration) createRandomSpansAndWrite(t *testing.T, traceID model.TraceID, numOfSpans int, startTime time.Time, duration time.Duration) []*model.Span {
	require.True(t, numOfSpans > 0)
	retSpans := make([]*model.Span, numOfSpans)
	for i := range retSpans {
		span := createRandomSpan(traceID, startTime, duration)
		err := s.writer.WriteSpan(span)
		require.NoError(t, err)
		retSpans[i] = span
	}
	return retSpans
}

func getTagsFromMap(tags map[string]string) model.KeyValues {
	if len(tags) == 0 {
		return model.KeyValues([]model.KeyValue{})
	}
	var retTags []model.KeyValue
	for key, value := range tags {
		retTags = append(retTags, tag(key, value))
	}
	return retTags
}

func tag(key string, value string) model.KeyValue {
	if value == "true" || value == "false" {
		return model.Bool(key, value == "true")
	}
	intVal, err := strconv.ParseInt(value, 10, 64)
	if err == nil {
		return model.Int64(key, intVal)
	}
	floatVal, err := strconv.ParseFloat(value, 64)
	if err == nil {
		return model.Float64(key, floatVal)
	}
	return model.String(key, value)
}

func (s *StorageIntegration) createTrace(t *testing.T, traceID model.TraceID, numOfSpans int, startTime time.Time, duration time.Duration) *model.Trace {
	assert.True(t, numOfSpans > 0)
	return &model.Trace{
		Spans: s.createRandomSpansAndWrite(t, traceID, numOfSpans, startTime, duration),
	}
}

func tracesMatch(traces []*model.Trace, numOfTraces int, numOfSpans int) bool {
	if len(traces) != numOfTraces {
		return false
	}
	for _, trace := range traces {
		if len(trace.Spans) != numOfSpans {
			return false
		}
	}
	return true
}

func (s *StorageIntegration) initializeTraces(t *testing.T, numOfTraces int, numOfSpans int) [][]*model.Trace {
	tracesBuckets := make([][]*model.Trace, len(differentQueries))
	traces := make([]*model.Trace, numOfTraces)
	for i := 0; i < numOfTraces; i++ {
		traces[i] = s.createRandomTrace(t, numOfSpans)
	}

	for _, trace := range traces {
		for i, query := range differentQueries {
			if CheckTraceWithQuery(trace, query) {
				tracesBuckets[i] = append(tracesBuckets[i], trace)
			}
		}
	}

	for _, traces := range tracesBuckets {
		if len(traces) == 0 {
			assert.FailNow(t, "A query is too limited to capture any traces, failing initialization...")
			return nil
		}
	}
	return tracesBuckets
}

func (s *StorageIntegration) createRandomTrace(t *testing.T, numOfSpans int) *model.Trace {
	randomStartTime, randomDuration := randomTimeAndDuration()
	return s.createTrace(t, model.TraceID{Low: uint64(rand.Uint32())}, numOfSpans, randomStartTime, randomDuration)
}

func (s *StorageIntegration) IntegrationTestGetServices(t *testing.T) {
	services := []string{"service1", "service2", "service3", "service4", "service5"}
	traceID := model.TraceID{Low: uint64(rand.Uint32())}
	for _, service := range services {
		randomStartTime, randomDuration := randomTimeAndDuration()
		span := createSpanWithParams(traceID, service, "op", randomStartTime, randomDuration)
		err := s.writer.WriteSpan(span)
		require.NoError(t, err)
	}

	s.refresh()

	var found bool
	iterations := 10 * timeOut
	for i := 0; i < iterations; i++ {
		s.logger.Info(fmt.Sprintf("Waiting for ES to update documents, iteration %d out of %d", i+1, iterations))
		actual, err := s.reader.GetServices()
		require.NoError(t, err)
		if found = assert.ObjectsAreEqualValues(services, actual); found {
			break
		}
		time.Sleep(100 * time.Millisecond) // Will wait up to 3 seconds at worst.
	}

	assert.True(t, found)
	assert.NoError(t, s.cleanUp())
}

func (s *StorageIntegration) IntegrationTestGetOperations(t *testing.T) {
	numOfServices := int64(3)
	numOfOperations := int64(5)

	traceID := model.TraceID{Low: uint64(rand.Uint32())}
	for i := int64(0); i < numOfServices; i++ {
		service := "service" + strconv.FormatInt(i, 10)
		for j := int64(0); j < numOfOperations; j++ {
			operation := "op" + strconv.FormatInt(i, 10) + strconv.FormatInt(j, 10)
			randomStartTime, randomDuration := randomTimeAndDuration()
			span := createSpanWithParams(traceID, service, operation, randomStartTime, randomDuration)
			err := s.writer.WriteSpan(span)
			require.NoError(t, err)
		}
	}

	s.refresh()

	var found bool
	iterations := 10 * timeOut
	expected := make([]string, numOfOperations)
	for i := range expected {
		expected[i] = "op0" + strconv.FormatInt(int64(i), 10)
	}
	for i := 0; i < iterations; i++ {
		s.logger.Info(fmt.Sprintf("Waiting for ES to update documents, iteration %d out of %d", i+1, iterations))
		actual, err := s.reader.GetOperations("service0")
		require.NoError(t, err)
		if found = assert.ObjectsAreEqualValues(expected, actual); found {
			break
		}
		time.Sleep(100 * time.Millisecond) // Will wait up to 3 seconds at worst.
	}

	assert.True(t, found)
	assert.NoError(t, s.cleanUp())
}

func (s *StorageIntegration) IntegrationTestGetTrace(t *testing.T) {
	traceID := model.TraceID{Low: uint64(rand.Uint32())}

	randomStartTime, randomDuration := randomTimeAndDuration()
	expected := s.createTrace(t, traceID, defaultNumSpans, randomStartTime, randomDuration)
	s.createRandomTrace(t, defaultNumSpans)

	s.refresh()

	var found bool
	iterations := 10 * timeOut
	for i := 0; i < iterations; i++ {
		s.logger.Info(fmt.Sprintf("Waiting for ES to update documents, iteration %d out of %d", i+1, iterations))
		actual, err := s.reader.GetTrace(traceID)
		if found = err == nil && len(actual.Spans) == defaultNumSpans; found {
			CompareModelTraces(t, expected, actual)
			break
		}
		time.Sleep(100 * time.Millisecond) // Will wait up to 3 seconds at worst.
	}

	assert.True(t, found)
	assert.NoError(t, s.cleanUp())
}

func (s *StorageIntegration) IntegrationTestFindTraces(t *testing.T) {
	for i := 1 ; i <= numOfQueries ; i++ {
		query, err := getQuery(i)
		require.NoError(t, err)
		t.Logf("Testing query case: %s ...", query.Caption)
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
		if found = err == nil && tracesMatch(actual, len(expected), defaultNumSpans); found {
			CompareListOfModelTraces(t, expected, actual)
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	assert.True(t, found)
}

func (s *StorageIntegration) writeTraces(traces []*model.Trace) error {
	for _, trace := range traces {
		for _, span := range trace.Spans {
			err := s.writer.WriteSpan(span)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func getSubsetOfTraces(traces []*model.Trace, expectedTraces []int) []*model.Trace {
	retTraces := make([]*model.Trace, len(expectedTraces))
	for i, traceNum := range expectedTraces {
		retTraces[i] = traces[traceNum+1]
	}
	return retTraces
}

func getTraceFixtures() ([]*model.Trace, error) {
	traces := make([]*model.Trace, numOfTraceFixtures)
	for i := 0; i < numOfTraceFixtures; i++ {
		trace, err := getTraceFixture(i+1)
		if err != nil {
			return nil, err
		}
		traces[i] = trace
	}
	return traces, nil
}

func getTraceFixture(i int) (*model.Trace, error) {
	fileName := fmt.Sprintf("fixtures/traces/trace_%02d.json", i)
	inStr, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, err
	}
	var trace model.Trace
	err = json.Unmarshal(inStr, &trace)
	if err != nil {
		return nil, err
	}
	return &trace, nil
}

func getQuery(i int) (*QueryToExpected, error) {
	fileName := fmt.Sprintf("fixtures/queries/query_%02d.json", i)
	inStr, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, err
	}
	var queryToExpected QueryToExpected
	err = json.Unmarshal(inStr, &queryToExpected)
	if err != nil {
		return nil, err
	}
	return &queryToExpected, nil
}

// DO NOT RUN IF YOU HAVE IMPORTANT SPANS IN ELASTICSEARCH
func (s *StorageIntegration) IntegrationTestAll(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	s.IntegrationTestGetServices(t)
	s.IntegrationTestGetOperations(t)
	s.IntegrationTestGetTrace(t)
	s.IntegrationTestFindTraces(t)
}












func Test_createstuff(t *testing.T) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "  ")

	enc.Encode(&QueryToExpected{Query: &spanstore.TraceQueryParameters{},ExpectedTraces:[]int{1,2,3}})

	ioutil.WriteFile("fixtures/queries/query_01.json", buf.Bytes(), 0644)
}