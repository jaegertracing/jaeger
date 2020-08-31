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

package spanstore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/uber/jaeger-lib/metrics/metricstest"
	"go.uber.org/atomic"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/cassandra/mocks"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra/spanstore/dbmodel"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

type spanWriterTest struct {
	session   *mocks.Session
	logger    *zap.Logger
	logBuffer *testutils.Buffer
	writer    *SpanWriter
}

func withSpanWriter(writeCacheTTL time.Duration, fn func(w *spanWriterTest), options ...Option,
) {
	session := &mocks.Session{}
	query := &mocks.Query{}
	session.On("Query",
		fmt.Sprintf(tableCheckStmt, schemas[latestVersion].tableName),
		mock.Anything).Return(query)
	query.On("Exec").Return(nil)
	logger, logBuffer := testutils.NewLogger()
	metricsFactory := metricstest.NewFactory(0)
	w := &spanWriterTest{
		session:   session,
		logger:    logger,
		logBuffer: logBuffer,
		writer:    NewSpanWriter(session, writeCacheTTL, metricsFactory, logger, options...),
	}
	fn(w)
}

var _ spanstore.Writer = &SpanWriter{} // check API conformance

func TestClientClose(t *testing.T) {
	withSpanWriter(0, func(w *spanWriterTest) {
		w.session.On("Close").Return(nil)
		w.writer.Close()
		w.session.AssertNumberOfCalls(t, "Close", 1)
	})
}

func TestSpanWriter(t *testing.T) {
	testCases := []struct {
		caption                        string
		firehose                       bool
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
			caption:  "main firehose query",
			firehose: true,
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
				`"trace_id":"0000000000000001"`,
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
			caption:                        "add span to operation name index",
			serviceOperationNameQueryError: errors.New("serviceOperationNameQueryError"),
			expectedError:                  "Failed to index operation name: failed to Exec query 'select from service_operation_index': serviceOperationNameQueryError",
			expectedLogs: []string{
				`"msg":"Failed to exec query"`,
				`"query":"select from service_operation_index"`,
				`"error":"serviceOperationNameQueryError"`,
			},
		},
		{
			caption:                       "add duration with no operation name",
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
					TraceID:       model.NewTraceID(0, 1),
					OperationName: "operation-a",
					Tags: model.KeyValues{
						model.String("x", "y"),
						model.String("json", `{"x":"y"}`), // string tag with json value will not be inserted
					},
					Process: &model.Process{
						ServiceName: "service-a",
					},
				}
				if testCase.firehose {
					span.Flags = model.FirehoseFlag
				}

				spanQuery := &mocks.Query{}
				spanQuery.On("Bind", matchEverything()).Return(spanQuery)
				spanQuery.On("Exec").Return(testCase.mainQueryError)
				spanQuery.On("String").Return("select from traces")

				serviceNameQuery := &mocks.Query{}
				serviceNameQuery.On("Bind", matchEverything()).Return(serviceNameQuery)
				serviceNameQuery.On("Exec").Return(testCase.serviceNameQueryError)
				serviceNameQuery.On("String").Return("select from service_name_index")

				serviceOperationNameQuery := &mocks.Query{}
				serviceOperationNameQuery.On("Bind", matchEverything()).Return(serviceOperationNameQuery)
				serviceOperationNameQuery.On("Exec").Return(testCase.serviceOperationNameQueryError)
				serviceOperationNameQuery.On("String").Return("select from service_operation_index")

				tagsQuery := &mocks.Query{}
				tagsQuery.On("Exec").Return(testCase.tagsQueryError)
				tagsQuery.On("String").Return("select from tags")

				durationNoOperationQuery := &mocks.Query{}
				durationNoOperationQuery.On("Bind", matchEverything()).Return(durationNoOperationQuery)
				durationNoOperationQuery.On("Exec").Return(testCase.durationNoOperationQueryError)
				durationNoOperationQuery.On("String").Return("select from duration_index")

				// Define expected queries
				w.session.On("Query", stringMatcher(insertSpan), matchEverything()).Return(spanQuery)
				w.session.On("Query", stringMatcher(serviceNameIndex), matchEverything()).Return(serviceNameQuery)
				w.session.On("Query", stringMatcher(serviceOperationIndex), matchEverything()).Return(serviceOperationNameQuery)
				// note: using matchOnce below because we only want one tag to be inserted
				w.session.On("Query", stringMatcher(tagIndex), matchOnce()).Return(tagsQuery)
				w.session.On("Query", stringMatcher(durationIndex), matchOnce()).Return(durationNoOperationQuery)

				w.writer.serviceNamesWriter = func(serviceName string) error { return testCase.serviceNameError }
				w.writer.operationNamesWriter = func(operation dbmodel.Operation) error { return testCase.serviceNameError }
				err := w.writer.WriteSpan(context.Background(), span)

				if testCase.expectedError == "" {
					assert.NoError(t, err)
				} else {
					assert.EqualError(t, err, testCase.expectedError)
				}
				for _, expectedLog := range testCase.expectedLogs {
					assert.Contains(t, w.logBuffer.String(), expectedLog)
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
			operationNamesWriter: func(operation dbmodel.Operation) error { return nil },
		},
		{
			serviceNamesWriter:   func(serviceName string) error { return expectedErr },
			operationNamesWriter: func(operation dbmodel.Operation) error { return nil },
			expectedError:        "some error",
		},
		{
			serviceNamesWriter:   func(serviceName string) error { return nil },
			operationNamesWriter: func(operation dbmodel.Operation) error { return expectedErr },
			expectedError:        "some error",
		},
	}
	for _, tc := range testCases {
		testCase := tc // capture loop var
		withSpanWriter(0, func(w *spanWriterTest) {
			w.writer.serviceNamesWriter = testCase.serviceNamesWriter
			w.writer.operationNamesWriter = testCase.operationNamesWriter
			err := w.writer.saveServiceNameAndOperationName(
				dbmodel.Operation{
					ServiceName:   "service",
					OperationName: "operation",
				})
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

func TestStorageMode_IndexOnly(t *testing.T) {
	withSpanWriter(0, func(w *spanWriterTest) {

		w.writer.serviceNamesWriter = func(serviceName string) error { return nil }
		w.writer.operationNamesWriter = func(operation dbmodel.Operation) error { return nil }
		span := &model.Span{
			TraceID: model.NewTraceID(0, 1),
			Process: &model.Process{
				ServiceName: "service-a",
			},
		}

		serviceNameQuery := &mocks.Query{}
		serviceNameQuery.On("Bind", matchEverything()).Return(serviceNameQuery)
		serviceNameQuery.On("Exec").Return(nil)

		serviceOperationNameQuery := &mocks.Query{}
		serviceOperationNameQuery.On("Bind", matchEverything()).Return(serviceOperationNameQuery)
		serviceOperationNameQuery.On("Exec").Return(nil)

		durationNoOperationQuery := &mocks.Query{}
		durationNoOperationQuery.On("Bind", matchEverything()).Return(durationNoOperationQuery)
		durationNoOperationQuery.On("Exec").Return(nil)

		w.session.On("Query", stringMatcher(serviceNameIndex), matchEverything()).Return(serviceNameQuery)
		w.session.On("Query", stringMatcher(serviceOperationIndex), matchEverything()).Return(serviceOperationNameQuery)
		w.session.On("Query", stringMatcher(durationIndex), matchOnce()).Return(durationNoOperationQuery)

		err := w.writer.WriteSpan(context.Background(), span)

		assert.NoError(t, err)
		serviceNameQuery.AssertExpectations(t)
		serviceOperationNameQuery.AssertExpectations(t)
		durationNoOperationQuery.AssertExpectations(t)
		w.session.AssertExpectations(t)
		w.session.AssertNotCalled(t, "Query", stringMatcher(insertSpan), matchEverything())
	}, StoreIndexesOnly())
}

var filterEverything = func(*dbmodel.Span, int) bool {
	return false
}

func TestStorageMode_IndexOnly_WithFilter(t *testing.T) {
	withSpanWriter(0, func(w *spanWriterTest) {
		w.writer.indexFilter = filterEverything
		w.writer.serviceNamesWriter = func(serviceName string) error { return nil }
		w.writer.operationNamesWriter = func(operation dbmodel.Operation) error { return nil }
		span := &model.Span{
			TraceID: model.NewTraceID(0, 1),
			Process: &model.Process{
				ServiceName: "service-a",
			},
		}
		err := w.writer.WriteSpan(context.Background(), span)
		assert.NoError(t, err)
		w.session.AssertExpectations(t)
		w.session.AssertNotCalled(t, "Query", stringMatcher(serviceOperationIndex), matchEverything())
		w.session.AssertNotCalled(t, "Query", stringMatcher(serviceNameIndex), matchEverything())
		w.session.AssertNotCalled(t, "Query", stringMatcher(durationIndex), matchEverything())
	}, StoreIndexesOnly())
}

func TestStorageMode_IndexOnly_FirehoseSpan(t *testing.T) {
	withSpanWriter(0, func(w *spanWriterTest) {
		serviceWritten := atomic.NewString("")
		operationWritten := &atomic.Value{}
		w.writer.serviceNamesWriter = func(serviceName string) error {
			serviceWritten.Store(serviceName)
			return nil
		}
		w.writer.operationNamesWriter = func(operation dbmodel.Operation) error {
			operationWritten.Store(operation)
			return nil
		}
		span := &model.Span{
			TraceID:       model.NewTraceID(0, 1),
			OperationName: "package-delivery",
			Process: &model.Process{
				ServiceName: "planet-express",
			},
			Flags: model.Flags(8),
		}

		serviceNameQuery := &mocks.Query{}
		serviceNameQuery.On("Bind", matchEverything()).Return(serviceNameQuery)
		serviceNameQuery.On("Exec").Return(nil)
		serviceNameQuery.On("String").Return("select from service_name_index")

		serviceOperationNameQuery := &mocks.Query{}
		serviceOperationNameQuery.On("Bind", matchEverything()).Return(serviceOperationNameQuery)
		serviceOperationNameQuery.On("Exec").Return(nil)
		serviceOperationNameQuery.On("String").Return("select from service_operation_index")

		// Define expected queries
		w.session.On("Query", stringMatcher(serviceNameIndex), matchEverything()).Return(serviceNameQuery)
		w.session.On("Query", stringMatcher(serviceOperationIndex), matchEverything()).Return(serviceOperationNameQuery)

		err := w.writer.WriteSpan(context.Background(), span)
		assert.NoError(t, err)
		w.session.AssertExpectations(t)
		w.session.AssertNotCalled(t, "Query", stringMatcher(tagIndex), matchEverything())
		w.session.AssertNotCalled(t, "Query", stringMatcher(durationIndex), matchEverything())
		assert.Equal(t, "planet-express", serviceWritten.Load())
		assert.Equal(t, dbmodel.Operation{
			ServiceName:   "planet-express",
			SpanKind:      "",
			OperationName: "package-delivery",
		}, operationWritten.Load())
	}, StoreIndexesOnly())
}

func TestStorageMode_StoreWithoutIndexing(t *testing.T) {
	withSpanWriter(0, func(w *spanWriterTest) {

		w.writer.serviceNamesWriter =
			func(serviceName string) error {
				assert.Fail(t, "Non indexing store shouldn't index")
				return nil
			}
		span := &model.Span{
			TraceID: model.NewTraceID(0, 1),
			Process: &model.Process{
				ServiceName: "service-a",
			},
		}
		spanQuery := &mocks.Query{}
		spanQuery.On("Exec").Return(nil)
		w.session.On("Query", stringMatcher(insertSpan), matchEverything()).Return(spanQuery)

		err := w.writer.WriteSpan(context.Background(), span)

		assert.NoError(t, err)
		spanQuery.AssertExpectations(t)
		w.session.AssertExpectations(t)
		w.session.AssertNotCalled(t, "Query", stringMatcher(serviceNameIndex), matchEverything())
	}, StoreWithoutIndexing())
}
