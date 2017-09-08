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

	"github.com/olivere/elastic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	jModel "github.com/uber/jaeger/model/json"
	"github.com/uber/jaeger/pkg/es/mocks"
)

func TestWriteService(t *testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		indexService := &mocks.IndexService{}

		indexName := "jaeger-1995-04-21"
		indexService.On("Index", stringMatcher(indexName)).Return(indexService)
		indexService.On("Type", stringMatcher(serviceType)).Return(indexService)
		indexService.On("Id", stringMatcher("service|operation")).Return(indexService)
		indexService.On("BodyJson", mock.AnythingOfType("spanstore.Service")).Return(indexService)
		indexService.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(&elastic.IndexResponse{}, nil)

		w.client.On("Index").Return(indexService)

		jsonSpan := &jModel.Span{
			TraceID:       jModel.TraceID("1"),
			SpanID:        jModel.SpanID("0"),
			OperationName: "operation",
			Process: &jModel.Process{
				ServiceName: "service",
			},
		}

		err := w.writer.writeService(indexName, jsonSpan)
		require.NoError(t, err)

		indexService.AssertNumberOfCalls(t, "Do", 1)
		assert.Equal(t, "", w.logBuffer.String())

		// test that cache works, will call the index service only once.
		err = w.writer.writeService(indexName, jsonSpan)
		require.NoError(t, err)
		indexService.AssertNumberOfCalls(t, "Do", 1)
	})
}

func TestWriteServiceError(t *testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		indexService := &mocks.IndexService{}

		indexName := "jaeger-1995-04-21"
		indexService.On("Index", stringMatcher(indexName)).Return(indexService)
		indexService.On("Type", stringMatcher(serviceType)).Return(indexService)
		indexService.On("Id", stringMatcher("service|operation")).Return(indexService)
		indexService.On("BodyJson", mock.AnythingOfType("spanstore.Service")).Return(indexService)
		indexService.On("Do", mock.AnythingOfType("*context.emptyCtx")).Return(nil, errors.New("service insertion error"))

		w.client.On("Index").Return(indexService)

		jsonSpan := &jModel.Span{
			TraceID:       jModel.TraceID("1"),
			SpanID:        jModel.SpanID("0"),
			OperationName: "operation",
			Process: &jModel.Process{
				ServiceName: "service",
			},
		}

		err := w.writer.writeService(indexName, jsonSpan)
		assert.EqualError(t, err, "Failed to insert service:operation: service insertion error")

		indexService.AssertNumberOfCalls(t, "Do", 1)

		expectedLogs := []string{
			`"msg":"Failed to insert service:operation"`,
			`"trace_id":"1"`,
			`"span_id":"0"`,
			`"error":"service insertion error"`,
		}

		for _, expectedLog := range expectedLogs {
			assert.True(t, strings.Contains(w.logBuffer.String(), expectedLog), "Log must contain %s, but was %s", expectedLog, w.logBuffer.String())
		}
	})
}

func TestSpanReader_GetServices(t *testing.T) {
	testGet(servicesAggregation, t)
}

func TestSpanReader_GetOperations(t *testing.T) {
	testGet(operationsAggregation, t)
}
