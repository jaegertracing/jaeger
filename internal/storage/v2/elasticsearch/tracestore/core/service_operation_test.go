// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/clientbuilder"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/snapshottest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
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
		indexService.On("Add")

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

func TestWriteServiceError(*testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		indexService := &mocks.IndexService{}

		indexName := "jaeger-1995-04-21"
		serviceHash := "de3b5a8f1a79989d"

		indexService.On("Index", stringMatcher(indexName)).Return(indexService)
		indexService.On("Type", stringMatcher(serviceType)).Return(indexService)
		indexService.On("Id", stringMatcher(serviceHash)).Return(indexService)
		indexService.On("BodyJson", mock.AnythingOfType("dbmodel.Service")).Return(indexService)
		indexService.On("Add")

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

// TestServiceOperationRequestSnapshots freezes the exact wire format of the
// service/operation read+write path over the current olivere client. Only ES6
// differs (no rest_total_hits_as_int on searches; _type on writes); every other
// version emits the same request, so snapshots collapse to es6 / es7-9.os1-3.

// newDataClient builds a real es.Client for the given backend version, pointed at
// the recording server. Version is set explicitly so no ping is issued, and the
// bulk processor only flushes on Close.
func newDataClient(t *testing.T, url string, version es.BackendVersion) es.Client {
	cfg := &config.Configuration{
		Servers:            []string{url},
		Version:            uint(version),
		DisableHealthCheck: true,
		LogLevel:           "info",
		BulkProcessing:     config.BulkProcessing{MaxBytes: -1},
	}
	client, err := clientbuilder.NewClient(context.Background(), cfg, zap.NewNop(), metrics.NullFactory, nil)
	require.NoError(t, err)
	return client
}

// dataRecorder answers each request with an empty-but-valid response for its
// endpoint (search, msearch, or bulk), so operations complete without error
// while the request is captured.
func dataRecorder() *snapshottest.Recorder {
	return snapshottest.NewRecorder(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		switch {
		case strings.HasSuffix(r.URL.Path, "_bulk"):
			w.Write([]byte(`{"took":0,"errors":false,"items":[]}`))
		case strings.HasSuffix(r.URL.Path, "_msearch"):
			w.Write([]byte(`{"responses":[{"took":0,"hits":{"total":0,"hits":[]}}]}`))
		default:
			w.Write([]byte(`{"took":0,"hits":{"total":0,"hits":[]}}`))
		}
	})
}

func TestServiceOperationRequestSnapshots(t *testing.T) {
	const (
		readIndex  = "test-jaeger-service-read"
		writeIndex = "test-jaeger-service-write-000001"
	)
	span := &dbmodel.Span{
		OperationName: "test-operation",
		Process:       dbmodel.Process{ServiceName: "test-service"},
	}

	getServices := map[es.BackendVersion]string{}
	getOperations := map[es.BackendVersion]string{}
	writeService := map[es.BackendVersion]string{}

	for _, version := range es.AllVersions {
		rec := dataRecorder()
		server := httptest.NewServer(rec)
		t.Cleanup(server.Close)
		client := newDataClient(t, server.URL, version)
		// The write path closes the client inline to flush the bulk request; this
		// cleanup only runs if an earlier assertion aborts first, so the client is
		// never closed twice.
		clientClosed := false
		t.Cleanup(func() {
			if !clientClosed {
				_ = client.Close()
			}
		})
		sos := NewServiceOperationStorage(func() es.Client { return client }, zap.NewNop(), 0)
		ctx := context.Background()

		rec.Reset()
		_, err := sos.getServices(ctx, []string{readIndex}, 10)
		require.NoError(t, err)
		getServices[version] = rec.Marshal(t)

		rec.Reset()
		_, err = sos.getOperations(ctx, []string{readIndex}, "test-service", 10)
		require.NoError(t, err)
		getOperations[version] = rec.Marshal(t)

		rec.Reset()
		sos.Write(writeIndex, span)
		require.NoError(t, client.Close()) // flushes the bulk request
		clientClosed = true
		writeService[version] = rec.Marshal(t)
	}

	snapshottest.AssertByVersion(t, "testdata/get_services", getServices)
	snapshottest.AssertByVersion(t, "testdata/get_operations", getOperations)
	snapshottest.AssertByVersion(t, "testdata/write_service", writeService)
}
