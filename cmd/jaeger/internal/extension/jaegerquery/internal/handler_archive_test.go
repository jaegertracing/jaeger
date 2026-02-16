// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
)

func TestGetArchivedTrace_NotFound(t *testing.T) {
	mockReader := &tracestoremocks.Reader{}
	mockReader.On("GetTraces", mock.Anything, mock.Anything).
		Return(emptyIter()).Once()
	for _, tc := range []struct {
		name   string
		reader tracestore.Reader
	}{
		{
			name:   "nil",
			reader: nil,
		},
		{
			name:   "mock reader",
			reader: mockReader,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withTestServer(t, func(ts *testServer) {
				ts.traceReader.On("GetTraces", mock.Anything, mock.Anything).
					Return(emptyIter()).Once()
				var response structuredResponse
				err := getJSON(ts.server.URL+"/api/traces/"+mockTraceID.String(), &response)
				require.EqualError(t, err,
					`404 error from server: {"data":null,"total":0,"limit":0,"offset":0,"errors":[{"code":404,"msg":"trace not found"}]}`+"\n",
				)
			}, querysvc.QueryServiceOptions{
				ArchiveTraceReader: tc.reader,
			}) // nil is ok
		})
	}
}

func TestGetArchivedTraceSuccess(t *testing.T) {
	mockReader := &tracestoremocks.Reader{}
	mockReader.On("GetTraces", mock.Anything, mock.Anything).
		Return(tracesIter(makeMockPTrace())).Once()
	withTestServer(t, func(ts *testServer) {
		// make main reader return empty (not found)
		ts.traceReader.On("GetTraces", mock.Anything, mock.Anything).
			Return(emptyIter()).Once()
		var response structuredTraceResponse
		err := getJSON(ts.server.URL+"/api/traces/"+mockTraceID.String(), &response)
		require.NoError(t, err)
		assert.Empty(t, response.Errors)
		assert.Len(t, response.Traces, 1)
	}, querysvc.QueryServiceOptions{ArchiveTraceReader: mockReader})
}

// Test failure in parsing trace ID.
func TestArchiveTrace_BadTraceID(t *testing.T) {
	withTestServer(t, func(ts *testServer) {
		var response structuredResponse
		err := postJSON(ts.server.URL+"/api/archive/badtraceid", []string{}, &response)
		require.Error(t, err)
	}, querysvc.QueryServiceOptions{})
}

// Test return of 404 when trace is not found in APIHandler.archive
func TestArchiveTrace_TraceNotFound(t *testing.T) {
	mockReader := &tracestoremocks.Reader{}
	mockReader.On("GetTraces", mock.Anything, mock.Anything).
		Return(emptyIter()).Once()
	mockWriter := &tracestoremocks.Writer{}
	// Not actually going to write the trace, so no need to define mockWriter action
	withTestServer(t, func(ts *testServer) {
		// make main reader return empty (not found)
		ts.traceReader.On("GetTraces", mock.Anything, mock.Anything).
			Return(emptyIter()).Once()
		var response structuredResponse
		err := postJSON(ts.server.URL+"/api/archive/"+mockTraceID.String(), []string{}, &response)
		require.EqualError(t, err, `404 error from server: {"data":null,"total":0,"limit":0,"offset":0,"errors":[{"code":404,"msg":"trace not found"}]}`+"\n")
	}, querysvc.QueryServiceOptions{ArchiveTraceReader: mockReader, ArchiveTraceWriter: mockWriter})
}

func TestArchiveTrace_NoStorage(t *testing.T) {
	withTestServer(t, func(ts *testServer) {
		var response structuredResponse
		err := postJSON(ts.server.URL+"/api/archive/"+mockTraceID.String(), []string{}, &response)
		require.EqualError(t, err, `500 error from server: {"data":null,"total":0,"limit":0,"offset":0,"errors":[{"code":500,"msg":"archive span storage was not configured"}]}`+"\n")
	}, querysvc.QueryServiceOptions{})
}

func TestArchiveTrace_Success(t *testing.T) {
	mockWriter := &tracestoremocks.Writer{}
	mockWriter.On("WriteTraces", mock.Anything, mock.Anything).
		Return(nil).Once()
	withTestServer(t, func(ts *testServer) {
		ts.traceReader.On("GetTraces", mock.Anything, mock.Anything).
			Return(tracesIter(makeMockPTrace())).Once()
		var response structuredResponse
		err := postJSON(ts.server.URL+"/api/archive/"+mockTraceID.String(), []string{}, &response)
		require.NoError(t, err)
	}, querysvc.QueryServiceOptions{ArchiveTraceWriter: mockWriter})
}

func TestArchiveTrace_SuccessWithTimeWindow(t *testing.T) {
	mockWriter := &tracestoremocks.Writer{}
	mockWriter.On("WriteTraces", mock.Anything, mock.Anything).
		Return(nil).Once()
	withTestServer(t, func(ts *testServer) {
		ts.traceReader.On("GetTraces", mock.Anything, mock.MatchedBy(func(params []tracestore.GetTraceParams) bool {
			if len(params) != 1 {
				return false
			}
			p := params[0]
			return p.TraceID == v1adapter.FromV1TraceID(mockTraceID) &&
				p.Start.Equal(time.UnixMicro(1)) &&
				p.End.Equal(time.UnixMicro(2))
		})).Return(tracesIter(makeMockPTrace())).Once()
		var response structuredTraceResponse
		err := postJSON(ts.server.URL+"/api/archive/"+mockTraceID.String()+"?start=1&end=2", []string{}, &response)
		require.NoError(t, err)
	}, querysvc.QueryServiceOptions{ArchiveTraceWriter: mockWriter})
}

func TestArchiveTrace_WriteErrors(t *testing.T) {
	mockWriter := &tracestoremocks.Writer{}
	mockWriter.On("WriteTraces", mock.Anything, mock.Anything).
		Return(errors.New("cannot save")).Once()
	withTestServer(t, func(ts *testServer) {
		ts.traceReader.On("GetTraces", mock.Anything, mock.Anything).
			Return(tracesIter(makeMockPTrace())).Once()
		var response structuredResponse
		err := postJSON(ts.server.URL+"/api/archive/"+mockTraceID.String(), []string{}, &response)
		require.EqualError(t, err, `500 error from server: {"data":null,"total":0,"limit":0,"offset":0,"errors":[{"code":500,"msg":"cannot save"}]}`+"\n")
	}, querysvc.QueryServiceOptions{ArchiveTraceWriter: mockWriter})
}

func TestArchiveTrace_BadTimeWindow(t *testing.T) {
	testCases := []struct {
		name  string
		query string
	}{
		{
			name:  "Bad start time",
			query: "start=a",
		},
		{
			name:  "Bad end time",
			query: "end=b",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockWriter := &tracestoremocks.Writer{}
			withTestServer(t, func(ts *testServer) {
				var response structuredTraceResponse
				err := postJSON(ts.server.URL+"/api/archive/"+mockTraceID.String()+"?"+tc.query, []string{}, &response)
				require.Error(t, err)
				require.ErrorContains(t, err, "400 error from server")
				require.ErrorContains(t, err, "unable to parse param")
			}, querysvc.QueryServiceOptions{ArchiveTraceWriter: mockWriter})
		})
	}
}
