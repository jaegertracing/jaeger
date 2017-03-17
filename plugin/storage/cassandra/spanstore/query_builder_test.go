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

package spanstore

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uber/jaeger/storage/spanstore"
)

var (
	testServiceName   = "someServiceName"
	testOperationName = "someOperationName"
	someMinStartTime  = testTime(5)
	someMaxStartTime  = testTime(10)
	someMinDuration   = testDuration(15)
	someMaxDuration   = testDuration(20)
)

func testTime(v int) time.Time {
	return time.Unix(0, int64(v)*1000)
}

func testDuration(v int) time.Duration {
	return time.Microsecond * time.Duration(v)
}

func TestGoodQueryScenarios(t *testing.T) {
	testServiceName = "someServiceName"

	type expectedQueries struct {
		mainQuery       string
		mainQueryParams []interface{}
		tagQueries      []string
		tagQueryParams  [][]interface{}
	}

	testCases := []struct {
		queryParams     *spanstore.TraceQueryParameters
		expectedQueries expectedQueries
	}{
		{
			&spanstore.TraceQueryParameters{NumTraces: 25},
			expectedQueries{
				mainQuery:       "SELECT trace_id, span_id FROM traces LIMIT ? ALLOW FILTERING",
				mainQueryParams: []interface{}{25},
			},
		},
		{
			&spanstore.TraceQueryParameters{OperationName: testOperationName, NumTraces: 25},
			expectedQueries{
				mainQuery:       "SELECT trace_id, span_id FROM traces WHERE operation_name = ? LIMIT ? ALLOW FILTERING",
				mainQueryParams: []interface{}{"someOperationName", 25},
			},
		},
		{
			&spanstore.TraceQueryParameters{ServiceName: testServiceName, NumTraces: 25},
			expectedQueries{
				mainQuery:       "SELECT trace_id, span_id FROM traces WHERE service_name = ? LIMIT ? ALLOW FILTERING",
				mainQueryParams: []interface{}{"someServiceName", 25},
			},
		},
		{
			&spanstore.TraceQueryParameters{ServiceName: testServiceName, OperationName: testOperationName, NumTraces: 25},
			expectedQueries{
				mainQuery:       "SELECT trace_id, span_id FROM traces WHERE service_name = ? AND operation_name = ? LIMIT ? ALLOW FILTERING",
				mainQueryParams: []interface{}{"someServiceName", "someOperationName", 25},
			},
		},
		{
			&spanstore.TraceQueryParameters{ServiceName: testServiceName, Tags: map[string]string{"someTagKey": "someTagValue"}, NumTraces: 25},
			expectedQueries{
				mainQuery:       "SELECT trace_id, span_id FROM traces WHERE service_name = ? LIMIT ? ALLOW FILTERING",
				mainQueryParams: []interface{}{"someServiceName", 25},
				tagQueries:      []string{"SELECT trace_id, span_id FROM tag_index WHERE service_name = ? AND tag_key = ? AND tag_value = ?"},
				tagQueryParams:  [][]interface{}{{"someServiceName", "someTagKey", "someTagValue"}},
			},
		},
		{
			&spanstore.TraceQueryParameters{ServiceName: testServiceName, StartTimeMin: someMinStartTime, NumTraces: 25},
			expectedQueries{
				mainQuery:       "SELECT trace_id, span_id FROM traces WHERE service_name = ? AND start_time >= ? LIMIT ? ALLOW FILTERING",
				mainQueryParams: []interface{}{"someServiceName", int64(5), 25},
			},
		},
		{
			&spanstore.TraceQueryParameters{ServiceName: testServiceName, StartTimeMax: someMaxStartTime, NumTraces: 25},
			expectedQueries{
				mainQuery:       "SELECT trace_id, span_id FROM traces WHERE service_name = ? AND start_time <= ? LIMIT ? ALLOW FILTERING",
				mainQueryParams: []interface{}{"someServiceName", int64(10), 25},
			},
		},
		{
			&spanstore.TraceQueryParameters{ServiceName: testServiceName, StartTimeMin: someMinStartTime, StartTimeMax: someMaxStartTime, NumTraces: 25},
			expectedQueries{
				mainQuery:       "SELECT trace_id, span_id FROM traces WHERE service_name = ? AND start_time >= ? AND start_time <= ? LIMIT ? ALLOW FILTERING",
				mainQueryParams: []interface{}{"someServiceName", int64(5), int64(10), 25},
			},
		},
		{
			&spanstore.TraceQueryParameters{
				ServiceName: testServiceName, StartTimeMin: someMinStartTime, StartTimeMax: someMaxStartTime, DurationMin: someMinDuration, NumTraces: 25,
			},
			expectedQueries{
				mainQuery:       "SELECT trace_id, span_id FROM traces WHERE service_name = ? AND start_time >= ? AND start_time <= ? AND duration >= ? LIMIT ? ALLOW FILTERING",
				mainQueryParams: []interface{}{"someServiceName", int64(5), int64(10), uint64(15), 25},
			},
		},
		{
			&spanstore.TraceQueryParameters{
				ServiceName: testServiceName, StartTimeMin: someMinStartTime, StartTimeMax: someMaxStartTime, DurationMax: someMaxDuration, NumTraces: 25,
			},
			expectedQueries{
				mainQuery:       "SELECT trace_id, span_id FROM traces WHERE service_name = ? AND start_time >= ? AND start_time <= ? AND duration <= ? LIMIT ? ALLOW FILTERING",
				mainQueryParams: []interface{}{"someServiceName", int64(5), int64(10), uint64(20), 25},
			},
		},
		{
			&spanstore.TraceQueryParameters{
				ServiceName: testServiceName, StartTimeMin: someMinStartTime, StartTimeMax: someMaxStartTime, DurationMin: someMinDuration, DurationMax: someMaxDuration, NumTraces: 25,
			},
			expectedQueries{
				mainQuery:       "SELECT trace_id, span_id FROM traces WHERE service_name = ? AND start_time >= ? AND start_time <= ? AND duration >= ? AND duration <= ? LIMIT ? ALLOW FILTERING",
				mainQueryParams: []interface{}{"someServiceName", int64(5), int64(10), uint64(15), uint64(20), 25},
			},
		},
		{
			&spanstore.TraceQueryParameters{ServiceName: testServiceName, OperationName: testOperationName, StartTimeMin: someMinStartTime, StartTimeMax: someMaxStartTime, DurationMin: someMinDuration, DurationMax: someMaxDuration, NumTraces: 25},
			expectedQueries{
				mainQuery:       "SELECT trace_id, span_id FROM traces WHERE service_name = ? AND operation_name = ? AND start_time >= ? AND start_time <= ? AND duration >= ? AND duration <= ? LIMIT ? ALLOW FILTERING",
				mainQueryParams: []interface{}{"someServiceName", "someOperationName", int64(5), int64(10), uint64(15), uint64(20), 25},
			},
		},
	}
	for testNo, tt := range testCases {
		testCase := tt // capture loop var
		t.Run(fmt.Sprintf("Test #%d", testNo), func(t *testing.T) {
			query, err := BuildQueries(testCase.queryParams)
			require.NoError(t, err)
			assert.Equal(t, testCase.expectedQueries.mainQuery, query.MainQuery.QueryString())
			if testCase.expectedQueries.mainQueryParams != nil {
				assert.EqualValues(t, testCase.expectedQueries.mainQueryParams, query.MainQuery.Parameters)
			}
			if assert.Len(t, query.TagQueries, len(testCase.expectedQueries.tagQueries)) {
				for i, tag := range testCase.expectedQueries.tagQueries {
					assert.Equal(t, tag, query.TagQueries[i].QueryString())
					assert.Equal(t, testCase.expectedQueries.tagQueryParams[i], query.TagQueries[i].Parameters)
				}
			}
		})
	}
}

func TestErrorScenarios(t *testing.T) {
	testCases := []struct {
		queryParams *spanstore.TraceQueryParameters
		expectedErr error
	}{
		{
			&spanstore.TraceQueryParameters{ServiceName: testServiceName, StartTimeMin: someMaxStartTime, StartTimeMax: someMinStartTime},
			ErrStartTimeMinGreaterThanMax,
		},
		{
			&spanstore.TraceQueryParameters{ServiceName: testServiceName, DurationMin: someMaxDuration, DurationMax: someMinDuration},
			ErrDurationMinGreaterThanMax,
		},
		{
			&spanstore.TraceQueryParameters{Tags: map[string]string{"someTagKey": "someTagValue"}},
			ErrServiceNameNotSet,
		},
		{
			&spanstore.TraceQueryParameters{ServiceName: testServiceName},
			ErrNumTracesNotSet,
		},
	}
	for _, testCase := range testCases {
		query, err := BuildQueries(testCase.queryParams)
		assert.Nil(t, query)
		assert.EqualError(t, err, testCase.expectedErr.Error())
	}
}
