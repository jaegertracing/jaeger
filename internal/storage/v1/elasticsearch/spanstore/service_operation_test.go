// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"testing"

	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/mocks"
)

func TestWriteService(t *testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		indexService := &mocks.IndexService{}

		indexName := "jaeger-1995-04-21"
		serviceHash := "de3b5a8f1a79989d"

		indexService.On("Index", stringMatcher(indexName)).Return(indexService)
		indexService.On("Type", stringMatcher(serviceType)).Return(indexService)
		indexService.On("Id", stringMatcher(serviceHash)).Return(indexService)
		indexService.On("BodyJson", mock.AnythingOfType("dbmodel.Service")).Return(indexService)
		indexService.On("Add", mock.Anything)

		w.client.On("Index").Return(indexService)

		jsonSpan := &dbmodel.Span{
			TraceID:       dbmodel.TraceID("1"),
			SpanID:        dbmodel.SpanID("0"),
			OperationName: "operation",
			Process: dbmodel.Process{
				ServiceName: "service",
			},
		}

		w.writer.writeService(indexName, jsonSpan)

		indexService.AssertNumberOfCalls(t, "Add", 1)
		assert.Empty(t, w.logBuffer.String())

		// test that cache works, will call the index service only once.
		w.writer.writeService(indexName, jsonSpan)
		indexService.AssertNumberOfCalls(t, "Add", 1)
	})
}

func TestWriteService_DataStream(t *testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		// Enable Data Stream usage
		w.writer.serviceWriter = NewServiceOperationStorage(w.writer.client, w.writer.logger, 0, true).Write

		indexService := &mocks.IndexService{}
		indexName := "jaeger-service-ds"

		// For Data Streams, we expect:
		// 1. No Id() call (ES auto-generates)
		// 2. OpType("create")
		indexService.On("Index", stringMatcher(indexName)).Return(indexService)
		indexService.On("Type", stringMatcher(serviceType)).Return(indexService)
		indexService.On("BodyJson", mock.AnythingOfType("dbmodel.Service")).Return(indexService)
		// We expect "create" opType to be added
		indexService.On("Add", "create").Return(indexService)

		w.client.On("Index").Return(indexService)

		jsonSpan := &dbmodel.Span{
			OperationName: "operation",
			Process: dbmodel.Process{
				ServiceName: "service",
			},
		}

		w.writer.writeService(indexName, jsonSpan)

		indexService.AssertNumberOfCalls(t, "Add", 1)
		// Ensure Id() was NOT called
		indexService.AssertNotCalled(t, "Id")
	})
}

func TestWriteServiceError(*testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		indexService := &mocks.IndexService{}

		indexName := "jaeger-1995-04-21"
		serviceHash := "de3b5a8f1a79989d"

		indexService.On("Index", stringMatcher(indexName)).Return(indexService)
		indexService.On("Type", stringMatcher(serviceType)).Return(indexService)
		indexService.On("Id", stringMatcher(serviceHash)).Return(indexService)
		indexService.On("BodyJson", mock.AnythingOfType("dbmodel.Service")).Return(indexService)
		indexService.On("Add", mock.Anything)

		w.client.On("Index").Return(indexService)

		jsonSpan := &dbmodel.Span{
			TraceID:       dbmodel.TraceID("1"),
			SpanID:        dbmodel.SpanID("0"),
			OperationName: "operation",
			Process: dbmodel.Process{
				ServiceName: "service",
			},
		}

		w.writer.writeService(indexName, jsonSpan)
	})
}

func TestSpanReader_GetServices(t *testing.T) {
	testGet(servicesAggregation, t)
}

func TestSpanReader_GetOperations(t *testing.T) {
	testGet(operationsAggregation, t)
}

func TestSpanReader_GetServicesEmptyIndex(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		mockSearchService(r).
			Return(&elastic.SearchResult{}, nil)
		mockMultiSearchService(r).
			Return(&elastic.MultiSearchResult{
				Responses: []*elastic.SearchResult{},
			}, nil)
		services, err := r.reader.GetServices(context.Background())
		require.NoError(t, err)
		assert.Empty(t, services)
	})
}

func TestSpanReader_GetOperationsEmptyIndex(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		mockSearchService(r).
			Return(&elastic.SearchResult{}, nil)
		mockMultiSearchService(r).
			Return(&elastic.MultiSearchResult{
				Responses: []*elastic.SearchResult{},
			}, nil)
		services, err := r.reader.GetOperations(
			context.Background(),
			dbmodel.OperationQueryParameters{ServiceName: "foo"},
		)
		require.NoError(t, err)
		assert.Empty(t, services)
	})
}
