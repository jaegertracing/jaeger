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

	"github.com/olivere/elastic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/uber/jaeger/model/json"
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

		jsonSpan := &json.Span{
			TraceID:       json.TraceID("1"),
			SpanID:        json.SpanID("0"),
			OperationName: "operation",
			Process: &json.Process{
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

		jsonSpan := &json.Span{
			TraceID:       json.TraceID("1"),
			SpanID:        json.SpanID("0"),
			OperationName: "operation",
			Process: &json.Process{
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
