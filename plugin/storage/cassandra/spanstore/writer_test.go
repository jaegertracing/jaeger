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

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/pkg/cassandra/mocks"
	"github.com/uber/jaeger/pkg/testutils"
	"github.com/uber/jaeger/plugin/storage/cassandra/spanstore/dbmodel"
	"github.com/uber/jaeger/storage/spanstore"
)

type spanWriterTest struct {
	session   *mocks.Session
	logger    *zap.Logger
	logBuffer *testutils.Buffer
	writer    *SpanWriter
}

func withSpanWriter(writeCacheTTL time.Duration, fn func(w *spanWriterTest)) {
	session := &mocks.Session{}
	logger, logBuffer := testutils.NewLogger()
	metricsFactory := metrics.NewLocalFactory(0)
	w := &spanWriterTest{
		session:   session,
		logger:    logger,
		logBuffer: logBuffer,
		writer:    NewSpanWriter(session, writeCacheTTL, metricsFactory, logger),
	}
	fn(w)
}

var _ spanstore.Writer = &SpanWriter{} // check API conformance

func TestSpanWriter(t *testing.T) {
	testCases := []struct {
		caption                        string
		mainQueryError                 error
		tagsQueryError                 error
		serviceNameQueryError          error
		serviceOperationNameQueryError error
		durationNoOperationQueryError  error
		serviceNameError               error
		expectedError                  string
		expectedLogs                   []string
	}{
		{
			caption: "main query",
		},
		{
			caption:        "main query error",
			mainQueryError: errors.New("main query error"),
			expectedError:  "Failed to insert span: failed to Exec query 'select from traces': main query error",
			expectedLogs: []string{
				`"msg":"Failed to exec query"`,
				`"query":"select from traces"`,
				`"error":"main query error"`,
				"Failed to insert span",
				`"trace_id":"1"`,
				`"span_id":0`,
			},
		},
		{
			caption:        "tags query error",
			tagsQueryError: errors.New("tags query error"),
			expectedError:  "Failed to index tags: Failed to index tag: failed to Exec query 'select from tags': tags query error",
			expectedLogs: []string{
				`"msg":"Failed to exec query"`,
				`"query":"select from tags"`,
				`"error":"tags query error"`,
				"Failed to index tags",
				`"tag_key":"x"`,
				`"tag_value":"y"`,
			},
		},
		{
			caption:          "save service name query error",
			serviceNameError: errors.New("serviceNameError"),
			expectedError:    "Failed to insert service name and operation name: serviceNameError",
			expectedLogs: []string{
				"Failed to insert service name and operation name",
			},
		},
		{
			caption:               "add span to service name index",
			serviceNameQueryError: errors.New("serviceNameQueryError"),
			expectedError:         "Failed to index service name: failed to Exec query 'select from service_name_index': serviceNameQueryError",
			expectedLogs: []string{
				`"msg":"Failed to exec query"`,
				`"query":"select from service_name_index"`,
				`"error":"serviceNameQueryError"`,
			},
		},
		{
			caption: "add span to operation name index",
			serviceOperationNameQueryError: errors.New("serviceOperationNameQueryError"),
			expectedError:                  "Failed to index operation name: failed to Exec query 'select from service_operation_index': serviceOperationNameQueryError",
			expectedLogs: []string{
				`"msg":"Failed to exec query"`,
				`"query":"select from service_operation_index"`,
				`"error":"serviceOperationNameQueryError"`,
			},
		},
		{
			caption: "add duration with no operation name",
			durationNoOperationQueryError: errors.New("durationNoOperationError"),
			expectedError:                 "Failed to index duration: failed to Exec query 'select from duration_index': durationNoOperationError",
			expectedLogs: []string{
				`"msg":"Failed to exec query"`,
				`"query":"select from duration_index"`,
				`"error":"durationNoOperationError"`,
			},
		},
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(testCase.caption, func(t *testing.T) {
			withSpanWriter(0, func(w *spanWriterTest) {
				span := &model.Span{
					TraceID:       model.TraceID{Low: 1},
					OperationName: "operation-a",
					Tags: model.KeyValues{
						model.String("x", "y"),
						model.String("json", `{"x":"y"}`), // string tag with json value will not be inserted
					},
					Process: &model.Process{
						ServiceName: "service-a",
					},
				}

				spanQuery := &mocks.Query{}
				spanQuery.On("Bind", matchEverything()).Return(spanQuery)
				spanQuery.On("Exec").Return(testCase.mainQueryError)
				spanQuery.On("String").Return("select from traces")

				tagsQuery := &mocks.Query{}
				tagsQuery.On("Exec").Return(testCase.tagsQueryError)
				tagsQuery.On("String").Return("select from tags")

				serviceNameQuery := &mocks.Query{}
				serviceNameQuery.On("Bind", matchEverything()).Return(serviceNameQuery)
				serviceNameQuery.On("Exec").Return(testCase.serviceNameQueryError)
				serviceNameQuery.On("String").Return("select from service_name_index")

				serviceOperationNameQuery := &mocks.Query{}
				serviceOperationNameQuery.On("Bind", matchEverything()).Return(serviceOperationNameQuery)
				serviceOperationNameQuery.On("Exec").Return(testCase.serviceOperationNameQueryError)
				serviceOperationNameQuery.On("String").Return("select from service_operation_index")

				durationNoOperationQuery := &mocks.Query{}
				durationNoOperationQuery.On("Bind", matchEverything()).Return(durationNoOperationQuery)
				durationNoOperationQuery.On("Exec").Return(testCase.durationNoOperationQueryError)
				durationNoOperationQuery.On("String").Return("select from duration_index")

				w.session.On("Query", stringMatcher(insertSpan), matchEverything()).Return(spanQuery)
				// note: using matchOnce below because we only want one tag to be inserted
				w.session.On("Query", stringMatcher(insertTag), matchOnce()).Return(tagsQuery)

				w.session.On("Query", stringMatcher(serviceNameIndex), matchEverything()).Return(serviceNameQuery)
				w.session.On("Query", stringMatcher(serviceOperationIndex), matchEverything()).Return(serviceOperationNameQuery)

				w.session.On("Query", stringMatcher(durationIndex), matchOnce()).Return(durationNoOperationQuery)

				w.writer.serviceNamesWriter = func(serviceName string) error { return testCase.serviceNameError }
				w.writer.operationNamesWriter = func(serviceName, operationName string) error { return testCase.serviceNameError }
				err := w.writer.WriteSpan(span)

				if testCase.expectedError == "" {
					assert.NoError(t, err)
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

func TestSpanWriterSaveServiceNameAndOperationName(t *testing.T) {
	expectedErr := errors.New("some error")
	testCases := []struct {
		serviceNamesWriter   serviceNamesWriter
		operationNamesWriter operationNamesWriter
		expectedError        string
	}{
		{
			serviceNamesWriter:   func(serviceName string) error { return nil },
			operationNamesWriter: func(serviceName, operationName string) error { return nil },
		},
		{
			serviceNamesWriter:   func(serviceName string) error { return expectedErr },
			operationNamesWriter: func(serviceName, operationName string) error { return nil },
			expectedError:        "some error",
		},
		{
			serviceNamesWriter:   func(serviceName string) error { return nil },
			operationNamesWriter: func(serviceName, operationName string) error { return expectedErr },
			expectedError:        "some error",
		},
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		withSpanWriter(0, func(w *spanWriterTest) {
			w.writer.serviceNamesWriter = testCase.serviceNamesWriter
			w.writer.operationNamesWriter = testCase.operationNamesWriter
			err := w.writer.saveServiceNameAndOperationName("service", "operation")
			if testCase.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, testCase.expectedError)
			}
		})
	}
}

func TestSpanWriterSkippingTags(t *testing.T) {
	longString := strings.Repeat("x", 300)
	testCases := []struct {
		key    string
		value  string
		insert bool
	}{
		{key: "x", value: "y", insert: true},
		{key: longString, value: "y", insert: false},
		{key: "x", value: longString, insert: false},
		{key: "x", value: `{"x":"y"}`, insert: false}, // value is a JSON
		{key: "x", value: `{"x":`, insert: true},      // value is not a JSON
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		withSpanWriter(0, func(w *spanWriterTest) {
			db := dbmodel.TagInsertion{
				ServiceName: "service-a",
				TagKey:      testCase.key,
				TagValue:    testCase.value,
			}
			ok := w.writer.shouldIndexTag(db)
			assert.Equal(t, testCase.insert, ok)
		})
	}
}
