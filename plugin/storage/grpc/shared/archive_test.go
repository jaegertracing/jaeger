// Copyright (c) 2020 The Jaeger Authors.
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

package shared

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/storage_v1"
	"github.com/jaegertracing/jaeger/proto-gen/storage_v1/mocks"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

func TestArchiveWriter_WriteSpan(t *testing.T) {
	mockSpan := &model.Span{
		TraceID: mockTraceID,
		SpanID:  model.NewSpanID(1),
		Process: &model.Process{},
	}

	archiveSpanWriter := new(mocks.ArchiveSpanWriterPluginClient)
	archiveSpanWriter.On("WriteArchiveSpan", mock.Anything, &storage_v1.WriteSpanRequest{Span: mockSpan}).
		Return(&storage_v1.WriteSpanResponse{}, nil)
	writer := &archiveWriter{client: archiveSpanWriter}

	err := writer.WriteSpan(context.Background(), mockSpan)
	assert.NoError(t, err)
}

func TestArchiveReader_GetTrace(t *testing.T) {
	mockTraceID := model.NewTraceID(0, 123456)
	mockSpan := model.Span{
		TraceID: mockTraceID,
		SpanID:  model.NewSpanID(1),
		Process: &model.Process{},
	}
	expected := &model.Trace{
		Spans: []*model.Span{&mockSpan},
	}

	traceClient := new(mocks.ArchiveSpanReaderPlugin_GetArchiveTraceClient)
	traceClient.On("Recv").Return(&storage_v1.SpansResponseChunk{
		Spans: []model.Span{mockSpan},
	}, nil).Once()
	traceClient.On("Recv").Return(nil, io.EOF)

	archiveSpanReader := new(mocks.ArchiveSpanReaderPluginClient)
	archiveSpanReader.On("GetArchiveTrace", mock.Anything, &storage_v1.GetTraceRequest{
		TraceID: mockTraceID,
	}).Return(traceClient, nil)
	reader := &archiveReader{client: archiveSpanReader}

	trace, err := reader.GetTrace(context.Background(), mockTraceID)
	assert.NoError(t, err)
	assert.Equal(t, expected, trace)
}

func TestArchiveReaderGetTrace_NoTrace(t *testing.T) {
	mockTraceID := model.NewTraceID(0, 123456)

	archiveSpanReader := new(mocks.ArchiveSpanReaderPluginClient)
	archiveSpanReader.On("GetArchiveTrace", mock.Anything, &storage_v1.GetTraceRequest{
		TraceID: mockTraceID,
	}).Return(nil, status.Errorf(codes.NotFound, ""))
	reader := &archiveReader{client: archiveSpanReader}

	_, err := reader.GetTrace(context.Background(), mockTraceID)
	assert.Equal(t, spanstore.ErrTraceNotFound, err)
}

func TestArchiveReader_FindTraceIDs(t *testing.T) {
	reader := archiveReader{client: &mocks.ArchiveSpanReaderPluginClient{}}
	_, err := reader.FindTraceIDs(context.Background(), nil)
	assert.Error(t, err)
}

func TestArchiveReader_FindTraces(t *testing.T) {
	reader := archiveReader{client: &mocks.ArchiveSpanReaderPluginClient{}}
	_, err := reader.FindTraces(context.Background(), nil)
	assert.Error(t, err)
}

func TestArchiveReader_GetOperations(t *testing.T) {
	reader := archiveReader{client: &mocks.ArchiveSpanReaderPluginClient{}}
	_, err := reader.GetOperations(context.Background(), spanstore.OperationQueryParameters{})
	assert.Error(t, err)
}

func TestArchiveReader_GetServices(t *testing.T) {
	reader := archiveReader{client: &mocks.ArchiveSpanReaderPluginClient{}}
	_, err := reader.GetServices(context.Background())
	assert.Error(t, err)
}
