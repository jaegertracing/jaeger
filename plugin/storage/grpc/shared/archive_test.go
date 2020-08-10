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

	archiveWriter := new(mocks.ArchiveSpanWriterPluginClient)
	archiveWriter.On("WriteArchiveSpan", mock.Anything, &storage_v1.WriteSpanRequest{Span: mockSpan}).
		Return(&storage_v1.WriteSpanResponse{}, nil)
	writer := &ArchiveWriter{archiveWriter}

	err := writer.WriteSpan(mockSpan)
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

	archiveReader := new(mocks.ArchiveSpanReaderPluginClient)
	archiveReader.On("GetArchiveTrace", mock.Anything, &storage_v1.GetTraceRequest{
		TraceID: mockTraceID,
	}).Return(traceClient, nil)
	reader := &ArchiveReader{archiveReader}

	trace, err := reader.GetTrace(context.Background(), mockTraceID)
	assert.NoError(t, err)
	assert.Equal(t, expected, trace)
}

func TestArchiveReader_FindTraceIDs(t *testing.T) {
	assert.Panics(t, func() {
		reader := ArchiveReader{&mocks.ArchiveSpanReaderPluginClient{}}
		_, _ = reader.FindTraceIDs(context.Background(), nil)
	})
}

func TestArchiveReader_FindTraces(t *testing.T) {
	assert.Panics(t, func() {
		reader := ArchiveReader{&mocks.ArchiveSpanReaderPluginClient{}}
		_, _ = reader.FindTraces(context.Background(), nil)
	})
}

func TestArchiveReader_GetOperations(t *testing.T) {
	assert.Panics(t, func() {
		reader := ArchiveReader{&mocks.ArchiveSpanReaderPluginClient{}}
		_, _ = reader.GetOperations(context.Background(), spanstore.OperationQueryParameters{})
	})
}

func TestArchiveReader_GetServices(t *testing.T) {
	assert.Panics(t, func() {
		reader := ArchiveReader{&mocks.ArchiveSpanReaderPluginClient{}}
		_, _ = reader.GetServices(context.Background())
	})
}
