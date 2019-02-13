// Copyright (c) 2019 The Jaeger Authors.
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

package querysvc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/model/adjuster"
	depsmocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)


var (
	mockTraceID = model.NewTraceID(0, 123456)
	mockTrace   = &model.Trace{
		Spans: []*model.Span{
			{
				TraceID: mockTraceID,
				SpanID:  model.NewSpanID(1),
				Process: &model.Process{},
			},
			{
				TraceID: mockTraceID,
				SpanID:  model.NewSpanID(2),
				Process: &model.Process{},
			},
		},
		Warnings: []string{},
	}
)

func initializeTestServiceWithArchiveOptions() (*QueryService, *spanstoremocks.Reader, *depsmocks.Reader, *spanstoremocks.Reader, *spanstoremocks.Writer) {
	readStorage := &spanstoremocks.Reader{}
	dependencyStorage := &depsmocks.Reader{}
	archiveReadStorage := &spanstoremocks.Reader{}
	archiveWriteStorage := &spanstoremocks.Writer{}
	options := QueryServiceOptions{
		ArchiveSpanReader: archiveReadStorage,
		ArchiveSpanWriter: archiveWriteStorage,
	}
	qs := NewQueryService(readStorage, dependencyStorage, options)
	return qs, readStorage, dependencyStorage, archiveReadStorage, archiveWriteStorage
}

func initializeTestServiceWithAdjustOption() (*QueryService, *spanstoremocks.Reader, *depsmocks.Reader) {
	readStorage := &spanstoremocks.Reader{}
	dependencyStorage := &depsmocks.Reader{}
	options := QueryServiceOptions{
		Adjuster: adjuster.Sequence(StandardAdjusters...),
	}
	qs := NewQueryService(readStorage, dependencyStorage, options)
	return qs, readStorage, dependencyStorage
}

func initializeTestService() (*QueryService, *spanstoremocks.Reader, *depsmocks.Reader) {
	readStorage := &spanstoremocks.Reader{}
	dependencyStorage := &depsmocks.Reader{}
	qs := NewQueryService(readStorage, dependencyStorage, QueryServiceOptions{})
	return qs, readStorage, dependencyStorage
}

// Test QueryService.GetTrace()
func TestGetTraceSuccess(t *testing.T) {
	qs, readMock, _ := initializeTestService()
	readMock.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(mockTrace, nil).Once()

	ctx := context.Background()
	res, err := qs.GetTrace(context.WithValue(ctx, "foo", "bar"), mockTraceID)
	assert.NoError(t, err)
	assert.Equal(t, res, mockTrace)
}

// Test QueryService.GetTrace() without ArchiveSpanReader
func TestGetTraceNotFound(t *testing.T) {
	qs, readMock, _ := initializeTestService()
	readMock.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(nil, spanstore.ErrTraceNotFound).Once()

	ctx := context.Background()
	_, err := qs.GetTrace(context.WithValue(ctx, "foo", "bar"), mockTraceID)
	assert.Equal(t, err, spanstore.ErrTraceNotFound)
}

// Test QueryService.GetTrace() with ArchiveSpanReader
func TestGetTraceFromArchiveStorage(t *testing.T) {
	qs, readMock, _, readArchiveMock, _ := initializeTestServiceWithArchiveOptions()
	readMock.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(nil, spanstore.ErrTraceNotFound).Once()
	readArchiveMock.On("GetTrace", mock.AnythingOfType("*context.valueCtx"), mock.AnythingOfType("model.TraceID")).
		Return(mockTrace, nil).Once()

	ctx := context.Background()
	res, err := qs.GetTrace(context.WithValue(ctx, "foo", "bar"), mockTraceID)
	assert.NoError(t, err)
	assert.Equal(t, res, mockTrace)
}
