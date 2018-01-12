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

package spanstore

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
	"gopkg.in/olivere/elastic.v5"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/model/json"
	"github.com/jaegertracing/jaeger/pkg/es/mocks"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

type spanWriterTest struct {
	client    *mocks.Client
	logger    *zap.Logger
	logBuffer *testutils.Buffer
	writer    *SpanWriter
}

func withSpanWriter(fn func(w *spanWriterTest)) {
	client := &mocks.Client{}
	logger, logBuffer := testutils.NewLogger()
	metricsFactory := metrics.NewLocalFactory(0)
	w := &spanWriterTest{
		client:    client,
		logger:    logger,
		logBuffer: logBuffer,
		writer:    NewSpanWriter(client, logger, metricsFactory, 0, 0),
	}
	fn(w)
}

var _ spanstore.Writer = &SpanWriter{} // check API conformance

// This test behaves as a large test that checks WriteSpan's behavior as a whole.
// Extra tests for individual functions are below.
func TestSpanWriter_WriteSpan(t *testing.T) {
	testCases := []struct {
		caption                 string
		serviceIndexExists      bool
		spanIndexExists         bool
		serviceIndexCreateError error
		spanIndexCreateError    error
		servicePutError         error
		spanPutError            error
		expectedError           string
		expectedLogs            []string
	}{
		{
			caption: "index exists query",

			serviceIndexExists: true,
			spanIndexExists:    true,

			expectedError: "",
			expectedLogs:  []string{},
		},
		{
			caption: "index dne/creation query",

			serviceIndexExists: false,

			expectedError: "",
			expectedLogs:  []string{},
		},
		{
			caption: "index creation error",

			serviceIndexExists: false,

			serviceIndexCreateError: errors.New("index creation error"),
			expectedError:           "Failed to create index: index creation error",
			expectedLogs: []string{
				`"msg":"Failed to create index"`,
				`"trace_id":"1"`,
				`"span_id":"0"`,
				`"error":"index creation error"`,
			},
		},
		{
			caption: "service insertion error",

			serviceIndexExists: false,

			servicePutError: errors.New("service insertion error"),
			expectedError:   "Failed to insert service:operation: service insertion error",
			expectedLogs: []string{
				`"msg":"Failed to insert service:operation"`,
				`"trace_id":"1"`,
				`"span_id":"0"`,
				`"error":"service insertion error"`,
			},
		},
		{
			caption: "span insertion error",

			serviceIndexExists: false,

			spanPutError:  errors.New("span insertion error"),
			expectedError: "Failed to insert span: span insertion error",
			expectedLogs: []string{
				`"msg":"Failed to insert span"`,
				`"trace_id":"1"`,
				`"span_id":"0"`,
				`"error":"span insertion error"`,
			},
		},
		{
			caption: "span index dne error",

			serviceIndexExists: true,
			spanIndexExists:    false,

			spanIndexCreateError: errors.New("span index creation error"),
			expectedError:        "Failed to create index: span index creation error",
			expectedLogs: []string{
				`"msg":"Failed to create index"`,
				`"trace_id":"1"`,
				`"span_id":"0"`,
				`"error":"span index creation error"`,
			},
		},
	}
	for _, tc := range testCases {
		testCase := tc
		t.Run(testCase.caption, func(t *testing.T) {
			withSpanWriter(func(w *spanWriterTest) {
				date, err := time.Parse(time.RFC3339, "1995-04-21T22:08:41+00:00")
				require.NoError(t, err)

				span := &model.Span{
					TraceID:       model.TraceID{Low: 1},
					SpanID:        model.SpanID(0),
					OperationName: "operation",
					Process: &model.Process{
						ServiceName: "service",
					},
					StartTime: date,
				}

				spanIndexName := "jaeger-span-1995-04-21"
				serviceIndexName := "jaeger-service-1995-04-21"

				serviceExistsService := &mocks.IndicesExistsService{}
				spanExistsService := &mocks.IndicesExistsService{}

				serviceExistsService.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(testCase.serviceIndexExists, nil)
				spanExistsService.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(testCase.spanIndexExists, nil)

				serviceCreateService := &mocks.IndicesCreateService{}
				serviceCreateService.On("Body", stringMatcher(w.writer.fixMapping(serviceMapping))).Return(serviceCreateService)
				serviceCreateService.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(nil, testCase.serviceIndexCreateError)

				spanCreateService := &mocks.IndicesCreateService{}
				spanCreateService.On("Body", stringMatcher(w.writer.fixMapping(spanMapping))).Return(spanCreateService)
				spanCreateService.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(nil, testCase.spanIndexCreateError)

				indexService := &mocks.IndexService{}
				indexServicePut := &mocks.IndexService{}
				indexSpanPut := &mocks.IndexService{}

				indexService.On("Index", stringMatcher(spanIndexName)).Return(indexService)
				indexService.On("Index", stringMatcher(serviceIndexName)).Return(indexService)

				indexService.On("Type", stringMatcher(serviceType)).Return(indexServicePut)
				indexService.On("Type", stringMatcher(spanType)).Return(indexSpanPut)

				indexServicePut.On("Id", stringMatcher("service|operation")).Return(indexServicePut)
				indexServicePut.On("BodyJson", mock.AnythingOfType("spanstore.Service")).Return(indexServicePut)
				indexServicePut.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(nil, testCase.servicePutError)

				indexSpanPut.On("Id", mock.AnythingOfType("string")).Return(indexSpanPut)
				indexSpanPut.On("BodyJson", mock.AnythingOfType("*spanstore.Span")).Return(indexSpanPut)
				indexSpanPut.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(nil, testCase.spanPutError)

				w.client.On("IndexExists", stringMatcher(spanIndexName)).Return(spanExistsService)
				w.client.On("CreateIndex", stringMatcher(spanIndexName)).Return(spanCreateService)
				w.client.On("IndexExists", stringMatcher(serviceIndexName)).Return(serviceExistsService)
				w.client.On("CreateIndex", stringMatcher(serviceIndexName)).Return(serviceCreateService)
				w.client.On("Index").Return(indexService)

				err = w.writer.WriteSpan(span)

				if testCase.expectedError == "" {
					assert.NoError(t, err)
					indexServicePut.AssertNumberOfCalls(t, "Do", 1)
					indexSpanPut.AssertNumberOfCalls(t, "Do", 1)
				} else {
					assert.EqualError(t, err, testCase.expectedError)
				}

				for _, expectedLog := range testCase.expectedLogs {
					assert.True(t, strings.Contains(w.logBuffer.String(), expectedLog), "Log must contain %s, but was %s", expectedLog, w.logBuffer.String())
				}
				if len(testCase.expectedLogs) == 0 {
					assert.Equal(t, "", w.logBuffer.String())
				}
			})
		})
	}
}

func TestSpanIndexName(t *testing.T) {
	date, err := time.Parse(time.RFC3339, "1995-04-21T22:08:41+00:00")
	require.NoError(t, err)
	span := &model.Span{
		StartTime: date,
	}
	spanIndexName, serviceIndexName := indexNames(span)
	assert.Equal(t, "jaeger-span-1995-04-21", spanIndexName)
	assert.Equal(t, "jaeger-service-1995-04-21", serviceIndexName)
}

func TestCheckAndCreateIndex(t *testing.T) {
	testCases := []struct {
		indexExists      bool
		indexExistsError error
		createResult     *elastic.IndicesCreateResult
		createError      error
		expectedError    string
		expectedLogs     []string
	}{
		{
			indexExists:  false,
			createResult: &elastic.IndicesCreateResult{},
		},
		{
			createError:   errors.New("index creation error"),
			expectedError: "Failed to create index: index creation error",
			expectedLogs: []string{
				`"msg":"Failed to create index"`,
				`"trace_id":"1"`,
				`"span_id":"0"`,
				`"error":"index creation error"`,
			},
		},
		{
			indexExists:      false,
			createError:      &elastic.Error{Details: &elastic.ErrorDetails{Type: "index_already_exists_exception"}},
			indexExistsError: &elastic.Error{Details: &elastic.ErrorDetails{Type: "index_already_exists_exception"}},
		},
	}
	for _, testCase := range testCases {
		withSpanWriter(func(w *spanWriterTest) {
			existsService := &mocks.IndicesExistsService{}
			existsService.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(testCase.indexExists, testCase.indexExistsError)

			createService := &mocks.IndicesCreateService{}
			createService.On("Body", stringMatcher(w.writer.fixMapping(spanMapping))).Return(createService)
			createService.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(testCase.createResult, testCase.createError)

			indexName := "jaeger-1995-04-21"
			w.client.On("IndexExists", stringMatcher(indexName)).Return(existsService)
			w.client.On("CreateIndex", stringMatcher(indexName)).Return(createService)

			jsonSpan := &json.Span{
				TraceID: json.TraceID("1"),
				SpanID:  json.SpanID("0"),
			}

			err := w.writer.createIndex(indexName, spanMapping, jsonSpan)
			createService.AssertNumberOfCalls(t, "Do", 1)

			if testCase.expectedError == "" {
				assert.NoError(t, err)
				// makes sure that the cache works
				_ = w.writer.createIndex(indexName, spanMapping, jsonSpan)
				createService.AssertNumberOfCalls(t, "Do", 1)
			} else {
				assert.EqualError(t, err, testCase.expectedError)
			}

			for _, expectedLog := range testCase.expectedLogs {
				assert.True(t, strings.Contains(w.logBuffer.String(), expectedLog), "Log must contain %s, but was %s", expectedLog, w.logBuffer.String())
			}
			if len(testCase.expectedLogs) == 0 {
				assert.Equal(t, "", w.logBuffer.String())
			}
		})
	}
}

func TestFixMapping(t *testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		testMapping := `{
		   "settings":{
		      "index.number_of_shards": ${__NUMBER_OF_SHARDS__},
      		      "index.number_of_replicas": ${__NUMBER_OF_REPLICAS__},
		      "index.mapping.nested_fields.limit":50,
		      "index.requests.cache.enable":true,
		      "index.mapper.dynamic":false
		   },
		   "mappings":{
		      "_default_":{
			 "_all":{
			    "enabled":false
			 }
		      }
		   }
		}`
		expectedMapping := `{
		   "settings":{
		      "index.number_of_shards": 5,
      		      "index.number_of_replicas": 1,
		      "index.mapping.nested_fields.limit":50,
		      "index.requests.cache.enable":true,
		      "index.mapper.dynamic":false
		   },
		   "mappings":{
		      "_default_":{
			 "_all":{
			    "enabled":false
			 }
		      }
		   }
		}`

		assert.Equal(t, expectedMapping, w.writer.fixMapping(testMapping))
	})
}

func TestWriteSpanInternal(t *testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		indexService := &mocks.IndexService{}

		indexName := "jaeger-1995-04-21"
		indexService.On("Index", stringMatcher(indexName)).Return(indexService)
		indexService.On("Type", stringMatcher(spanType)).Return(indexService)
		indexService.On("BodyJson", mock.AnythingOfType("*spanstore.Span")).Return(indexService)
		indexService.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(&elastic.IndexResponse{}, nil)

		w.client.On("Index").Return(indexService)

		jsonSpan := &json.Span{}

		err := w.writer.writeSpan(indexName, jsonSpan)
		require.NoError(t, err)

		indexService.AssertNumberOfCalls(t, "Do", 1)
		assert.Equal(t, "", w.logBuffer.String())
	})
}

func TestWriteSpanInternalError(t *testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		indexService := &mocks.IndexService{}

		indexName := "jaeger-1995-04-21"
		indexService.On("Index", stringMatcher(indexName)).Return(indexService)
		indexService.On("Type", stringMatcher(spanType)).Return(indexService)
		indexService.On("BodyJson", mock.AnythingOfType("*spanstore.Span")).Return(indexService)
		indexService.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(nil, errors.New("span insertion error"))

		w.client.On("Index").Return(indexService)

		jsonSpan := &json.Span{
			TraceID: json.TraceID("1"),
			SpanID:  json.SpanID("0"),
		}

		err := w.writer.writeSpan(indexName, jsonSpan)
		assert.EqualError(t, err, "Failed to insert span: span insertion error")

		indexService.AssertNumberOfCalls(t, "Do", 1)

		expectedLogs := []string{
			`"msg":"Failed to insert span"`,
			`"trace_id":"1"`,
			`"span_id":"0"`,
			`"error":"span insertion error"`,
		}

		for _, expectedLog := range expectedLogs {
			assert.True(t, strings.Contains(w.logBuffer.String(), expectedLog), "Log must contain %s, but was %s", expectedLog, w.logBuffer.String())
		}
	})
}

// stringMatcher can match a string argument when it contains a specific substring q
func stringMatcher(q string) interface{} {
	matchFunc := func(s string) bool {
		return strings.Contains(s, q)
	}
	return mock.MatchedBy(matchFunc)
}
