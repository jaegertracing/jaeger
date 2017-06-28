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
)

type QueryType int

type StorageIntegration struct {
	ctx     context.Context
	logger  *zap.Logger
	writer  spanstore.Writer
	reader  spanstore.Reader
	cleanUp func() error
	refresh func() error // function called between set-up and queries in each test
}

const (
	floatTagVal  = "95.0421"
	intTagVal    = "950421"
	stringTagVal = "xyz"
	boolTagVal   = "true"

	numOfInitTraces = 300
	defaultNumSpans = 5
	timeOut         = 3 // in seconds
)

var (
	// To add more various queries, add more TraceQueryParameters in differentQueries.
	// This should be sufficient; no need for extra code below.
	// Be sure to make the query large enough to capture some traces.
	// The below initializeTraces fn will fail if a query cannot capture traces.
	differentQueries = []*spanstore.TraceQueryParameters{
		{
			ServiceName:  randomService(),
			StartTimeMin: time.Now().Add(-3 * time.Hour),
			StartTimeMax: time.Now(),
			NumTraces:    numOfInitTraces, //set to numOfInitTraces if you don't care about number of traces retrieved, and you want all
		},
		{
			ServiceName:   randomService(),
			OperationName: randomOperation(),
			StartTimeMin:  time.Now().Add(-3 * time.Hour),
			StartTimeMax:  time.Now(),
			NumTraces:     numOfInitTraces,
		},
		{
			ServiceName: randomService(),
			Tags: map[string]string{
				"tag1": intTagVal,
				"tag3": stringTagVal,
			},
			StartTimeMin: time.Now().Add(-3 * time.Hour),
			StartTimeMax: time.Now(),
			NumTraces:    numOfInitTraces,
		},
		{
			ServiceName:  randomService(),
			StartTimeMin: time.Now().Add(-3 * time.Hour),
			StartTimeMax: time.Now(),
			DurationMin:  100 * time.Millisecond,
			DurationMax:  400 * time.Millisecond,
			NumTraces:    numOfInitTraces,
		},
		{
			ServiceName:  randomService(),
			StartTimeMin: time.Now().Add(-3 * time.Hour),
			StartTimeMax: time.Now(),
			NumTraces:    2,
		},
	}
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
)

func randomTimeAndDuration() (time.Time, time.Duration) {
	randomStartTime := time.Now().Add(time.Duration(-1*rand.Intn(80000)) * time.Second) // randomizing the startTime
	randomDuration := 200*time.Millisecond + time.Duration(rand.Intn(800))*time.Millisecond
	return randomStartTime, randomDuration
}

func randomTimeAndDurationBetween(startTime time.Time, duration time.Duration) (time.Time, time.Duration) {
	randomStartDuration := rand.Intn(int(duration))
	randomDuration := rand.Intn(int(duration) - randomStartDuration)
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
			if checkTraceWithQuery(trace, query) {
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

func checkTraceWithQuery(trace *model.Trace, traceQuery *spanstore.TraceQueryParameters) bool {
	for _, span := range trace.Spans {
		if checkSpanWithQuery(span, traceQuery) {
			return true
		}
	}
	return false
}

func checkSpanWithQuery(span *model.Span, traceQuery *spanstore.TraceQueryParameters) bool {
	return matchDurationQueryWithSpan(span, traceQuery) &&
		matchServiceNameQueryWithSpan(span, traceQuery) &&
		matchOperationNameQueryWithSpan(span, traceQuery) &&
		matchStartTimeQueryWithSpan(span, traceQuery) &&
		matchTagsQueryWithSpan(span, traceQuery)
}

func matchServiceNameQueryWithSpan(span *model.Span, traceQuery *spanstore.TraceQueryParameters) bool {
	return span.Process.ServiceName == traceQuery.ServiceName
}

func matchOperationNameQueryWithSpan(span *model.Span, traceQuery *spanstore.TraceQueryParameters) bool {
	return traceQuery.OperationName == "" || span.OperationName == traceQuery.OperationName
}

func matchStartTimeQueryWithSpan(span *model.Span, traceQuery *spanstore.TraceQueryParameters) bool {
	return traceQuery.StartTimeMin.Before(span.StartTime) && span.StartTime.Before(traceQuery.StartTimeMax)
}

func matchDurationQueryWithSpan(span *model.Span, traceQuery *spanstore.TraceQueryParameters) bool {
	if traceQuery.DurationMin == 0 && traceQuery.DurationMax == 0 {
		return true
	}
	return traceQuery.DurationMin <= span.Duration && span.Duration <= traceQuery.DurationMax
}

func matchTagsQueryWithSpan(span *model.Span, traceQuery *spanstore.TraceQueryParameters) bool {
	if len(traceQuery.Tags) == 0 {
		return true
	}
	return spanHasAllTags(span, traceQuery.Tags)
}

func spanHasAllTags(span *model.Span, tags map[string]string) bool {
	for key, val := range tags {
		if !checkAllSpots(span, key, val) {
			return false
		}
	}
	return true
}

func checkAllSpots(span *model.Span, key string, val string) bool {
	tag, found := span.Tags.FindByKey(key)
	if found && tag.AsString() == val {
		return true
	}
	if span.Process != nil {
		tag, found = span.Process.Tags.FindByKey(key)
		if found && tag.AsString() == val {
			return true
		}
	}
	for _, log := range span.Logs {
		if len(log.Fields) == 0 {
			continue
		}
		tag, found = model.KeyValues(log.Fields).FindByKey(key)
		if found && tag.AsString() == val {
			return true
		}
	}
	return false
}

func (s *StorageIntegration) ITestGetServices(t *testing.T) {
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

func (s *StorageIntegration) ITestGetOperations(t *testing.T) {
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

func (s *StorageIntegration) ITestGetTrace(t *testing.T) {
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

func (s *StorageIntegration) ITestFindTraces(t *testing.T) {
	queryTypeToExpectedOutput := s.initializeTraces(t, numOfInitTraces, defaultNumSpans)

	s.refresh()

	iterations := 10 * timeOut
	for queryType, expected := range queryTypeToExpectedOutput {
		var found bool
		for i := 0; i < iterations; i++ {
			s.logger.Info(fmt.Sprintf("Waiting for ES to update documents, iteration %d out of %d", i+1, iterations))
			traceQuery := differentQueries[queryType]
			actual, err := s.reader.FindTraces(traceQuery)

			require.NoError(t, err)
			if len(actual) == traceQuery.NumTraces {
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
		if !found {
			t.Log(queryType, "is weird")
		}
	}
	assert.NoError(t, s.cleanUp())
}

// DO NOT RUN IF YOU HAVE IMPORTANT SPANS IN ELASTICSEARCH
func (s *StorageIntegration) ITestAll(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	s.ITestGetServices(t)
	s.ITestGetOperations(t)
	s.ITestGetTrace(t)
	s.ITestFindTraces(t)
}
