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

package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared/mocks"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var (
	mockTraceID = model.NewTraceID(0, 123456)
	mockSpan    = &model.Span{
		TraceID: mockTraceID,
		SpanID:  model.NewSpanID(1),
		Process: &model.Process{},
	}
)

func TestArchiveWriter_WriteSpan(t *testing.T) {
	archiveWriter := new(mocks.ArchiveWriter)
	archiveWriter.On("WriteArchiveSpan", mockSpan).Return(nil)
	writer := &ArchiveWriter{impl: archiveWriter}

	err := writer.WriteSpan(mockSpan)
	assert.NoError(t, err)
}

func TestArchiveReader_GetTrace(t *testing.T) {
	expected := &model.Trace{
		Spans: []*model.Span{
			mockSpan,
		},
	}
	archiveReader := new(mocks.ArchiveReader)
	archiveReader.On("GetArchiveTrace", mock.Anything, mockTraceID).Return(expected, nil)
	reader := &ArchiveReader{impl: archiveReader}

	trace, err := reader.GetTrace(context.Background(), mockTraceID)
	assert.NoError(t, err)
	assert.Equal(t, expected, trace)
}

func TestArchiveReader_FindTraceIDs(t *testing.T) {
	assert.Panics(t, func() {
		reader := ArchiveReader{impl: &mocks.ArchiveReader{}}
		_, _ = reader.FindTraceIDs(context.Background(), nil)
	})
}

func TestArchiveReader_FindTraces(t *testing.T) {
	assert.Panics(t, func() {
		reader := ArchiveReader{impl: &mocks.ArchiveReader{}}
		_, _ = reader.FindTraces(context.Background(), nil)
	})
}

func TestArchiveReader_GetOperations(t *testing.T) {
	assert.Panics(t, func() {
		reader := ArchiveReader{impl: &mocks.ArchiveReader{}}
		_, _ = reader.GetOperations(context.Background(), spanstore.OperationQueryParameters{})
	})
}

func TestArchiveReader_GetServices(t *testing.T) {
	assert.Panics(t, func() {
		reader := ArchiveReader{impl: &mocks.ArchiveReader{}}
		_, _ = reader.GetServices(context.Background())
	})
}
