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
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/olivere/elastic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/pkg/es/mocks"
	"github.com/uber/jaeger/pkg/testutils"
	"github.com/uber/jaeger/storage/spanstore"
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
	w := &spanWriterTest{
		client:    client,
		logger:    logger,
		logBuffer: logBuffer,
		writer:    NewSpanWriter(client, logger),
	}
	fn(w)
}

func TestNewSpanWriter(t *testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		var writer spanstore.Writer = w.writer
		assert.NotNil(t, writer)
	})
}

func TestSpanWriter_WriteSpan(t *testing.T) {
	testCases := []struct {
		caption          string
		indexExists      bool
		indexExistsError error
		createResult     *elastic.IndicesCreateResult
		createError      error
		putResult        *elastic.IndexResponse
		servicePutError  error
		spanPutError     error
		expectedError    string
		expectedLogs     []string
	}{
		{
			caption: "main exists query",

			indexExists:  true,
			createResult: &elastic.IndicesCreateResult{},
			putResult:    &elastic.IndexResponse{},

			expectedError: "",
			expectedLogs:  []string{},
		},
		{
			caption: "main dne/creation query",

			indexExists:  false,
			createResult: &elastic.IndicesCreateResult{},
			putResult:    &elastic.IndexResponse{},

			expectedError: "",
			expectedLogs:  []string{},
		},
		{
			caption:      "index dne error",
			indexExists:  false,
			createResult: &elastic.IndicesCreateResult{},
			putResult:    &elastic.IndexResponse{},

			indexExistsError: errors.New("index dne error"),
			expectedError:    "Failed to find index: index dne error",
			expectedLogs: []string{
				`"msg":"Failed to find index"`,
				`"trace_id":"1"`,
				`"span_id":"0"`,
				`"error":"index dne error"`,
			},
		},
		{
			caption:      "index creation error",
			indexExists:  false,
			createResult: nil,
			putResult:    &elastic.IndexResponse{},

			indexExistsError: nil,
			createError:      errors.New("index creation error"),
			expectedError:    "Failed to create index: index creation error",
			expectedLogs: []string{
				`"msg":"Failed to create index"`,
				`"trace_id":"1"`,
				`"span_id":"0"`,
				`"error":"index creation error"`,
			},
		},
		{
			caption:      "service insertion error",
			indexExists:  false,
			createResult: &elastic.IndicesCreateResult{},
			putResult:    nil,

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
			caption:      "span insertion error",
			indexExists:  false,
			createResult: &elastic.IndicesCreateResult{},
			putResult:    nil,

			spanPutError:  errors.New("span insertion error"),
			expectedError: "Failed to insert span: span insertion error",
			expectedLogs: []string{
				`"msg":"Failed to insert span"`,
				`"trace_id":"1"`,
				`"span_id":"0"`,
				`"error":"span insertion error"`,
			},
		},
	}
	for _, tc := range testCases {
		testCase := tc
		t.Run(testCase.caption, func(t *testing.T) {
			withSpanWriter(func(w *spanWriterTest) {
				date, err := time.Parse(time.RFC3339, "1995-04-21T22:08:41+00:00")
				assert.NoError(t, err)

				span := &model.Span{
					TraceID:       model.TraceID{Low: 1},
					SpanID:        model.SpanID(0),
					OperationName: "operation",
					Process: &model.Process{
						ServiceName: "service",
					},
					StartTime:     date,
				}

				indexName := "jaeger-1995-04-21"

				existsService := &mocks.IndicesExistsService{}
				existsService.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(testCase.indexExists, testCase.indexExistsError)

				createService := &mocks.IndicesCreateService{}
				createService.On("Body", stringMatcher(spanMapping)).Return(createService)
				createService.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(testCase.createResult, testCase.createError)

				indexService := &mocks.IndexService{}
				indexServicePut := &mocks.IndexService{}
				indexSpanPut := &mocks.IndexService{}

				indexService.On("Index", stringMatcher(indexName)).Return(indexService)

				indexService.On("Type", stringMatcher(serviceType)).Return(indexServicePut)
				indexService.On("Type", stringMatcher(spanType)).Return(indexSpanPut)

				indexServicePut.On("Id", stringMatcher("service|operation")).Return(indexServicePut)
				indexServicePut.On("BodyJson", mock.AnythingOfType("Service")).Return(indexServicePut)
				indexServicePut.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(testCase.putResult, testCase.servicePutError)

				indexSpanPut.On("Id", mock.AnythingOfType("string")).Return(indexSpanPut)
				indexSpanPut.On("BodyJson", mock.AnythingOfType("*json.Span")).Return(indexSpanPut)
				indexSpanPut.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(testCase.putResult, testCase.spanPutError)


				w.client.On("IndexExists", stringMatcher(indexName)).Return(existsService)
				w.client.On("CreateIndex", stringMatcher(indexName)).Return(createService)
				w.client.On("Index").Return(indexService)

				err = w.writer.WriteSpan(span)

				if testCase.expectedError == "" {
					assert.NoError(t, err)
					if testCase.indexExists || testCase.indexExistsError != nil {
						createService.AssertNumberOfCalls(t, "Do", 0)
					} else {
						createService.AssertNumberOfCalls(t, "Do", 1)
					}
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

// stringMatcher can match a string argument when it contains a specific substring q
func stringMatcher(q string) interface{} {
	matchFunc := func(s string) bool {
		return strings.Contains(s, q)
	}
	return mock.MatchedBy(matchFunc)
}
