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

package app

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/storage/spanstore"
	spanstoremocks "github.com/uber/jaeger/storage/spanstore/mocks"
)

func TestGetArchivedTrace_NotFound(t *testing.T) {
	mockReader := &spanstoremocks.Reader{}
	mockReader.On("GetTrace", mock.AnythingOfType("model.TraceID")).
		Return(nil, spanstore.ErrTraceNotFound).Once()
	for _, tc := range []spanstore.Reader{nil, mockReader} {
		archiveReader := tc // capture loop var
		t.Run(fmt.Sprint(archiveReader), func(t *testing.T) {
			withTestServer(t, func(ts *testServer) {
				ts.spanReader.On("GetTrace", mock.AnythingOfType("model.TraceID")).
					Return(nil, spanstore.ErrTraceNotFound).Once()
				var response structuredResponse
				err := getJSON(ts.server.URL+"/api/traces/"+mockTraceID.String(), &response)
				assert.EqualError(t, err,
					`404 error from server: {"data":null,"total":0,"limit":0,"offset":0,"errors":[{"code":404,"msg":"trace not found"}]}`+"\n",
				)
			}, HandlerOptions.ArchiveSpanReader(archiveReader)) // nil is ok
		})
	}
}

func TestGetArchivedTraceSuccess(t *testing.T) {
	traceID := model.TraceID{Low: 123456}
	mockReader := &spanstoremocks.Reader{}
	mockReader.On("GetTrace", mock.AnythingOfType("model.TraceID")).
		Return(mockTrace, nil).Once()
	withTestServer(t, func(ts *testServer) {
		// maeke main reader return NotFound
		ts.spanReader.On("GetTrace", mock.AnythingOfType("model.TraceID")).
			Return(nil, spanstore.ErrTraceNotFound).Once()
		var response structuredTraceResponse
		err := getJSON(ts.server.URL+"/api/traces/"+mockTraceID.String(), &response)
		assert.NoError(t, err)
		assert.Len(t, response.Errors, 0)
		assert.Len(t, response.Traces, 1)
		assert.Equal(t, traceID.String(), string(response.Traces[0].TraceID))
	}, HandlerOptions.ArchiveSpanReader(mockReader))
}

func TestArchiveTrace_NoStorage(t *testing.T) {
	withTestServer(t, func(ts *testServer) {
		var response structuredResponse
		err := postJSON(ts.server.URL+"/api/archive/"+mockTraceID.String(), []string{}, &response)
		assert.EqualError(t, err, `500 error from server: {"data":null,"total":0,"limit":0,"offset":0,"errors":[{"code":500,"msg":"archive span storage was not configured"}]}`+"\n")
	})
}

func TestArchiveTrace_Success(t *testing.T) {
	mockWriter := &spanstoremocks.Writer{}
	mockWriter.On("WriteSpan", mock.AnythingOfType("*model.Span")).
		Return(nil).Times(2)
	withTestServer(t, func(ts *testServer) {
		ts.spanReader.On("GetTrace", mock.AnythingOfType("model.TraceID")).
			Return(mockTrace, nil).Once()
		var response structuredResponse
		err := postJSON(ts.server.URL+"/api/archive/"+mockTraceID.String(), []string{}, &response)
		assert.NoError(t, err)
	}, HandlerOptions.ArchiveSpanWriter(mockWriter))
}

func TestArchiveTrace_WriteErrors(t *testing.T) {
	mockWriter := &spanstoremocks.Writer{}
	mockWriter.On("WriteSpan", mock.AnythingOfType("*model.Span")).
		Return(errors.New("cannot save")).Times(2)
	withTestServer(t, func(ts *testServer) {
		ts.spanReader.On("GetTrace", mock.AnythingOfType("model.TraceID")).
			Return(mockTrace, nil).Once()
		var response structuredResponse
		err := postJSON(ts.server.URL+"/api/archive/"+mockTraceID.String(), []string{}, &response)
		assert.EqualError(t, err, `500 error from server: {"data":null,"total":0,"limit":0,"offset":0,"errors":[{"code":500,"msg":"[cannot save, cannot save]"}]}`+"\n")
	}, HandlerOptions.ArchiveSpanWriter(mockWriter))
}
