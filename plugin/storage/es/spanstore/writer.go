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
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/zap"

	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/model/converter/json"
	jModel "github.com/uber/jaeger/model/json"
	"github.com/uber/jaeger/pkg/es"
	storageMetrics "github.com/uber/jaeger/storage/spanstore/metrics"
)

const (
	spanType    = "span"
	serviceType = "service"
)

type spanWriterMetrics struct {
	exists           *storageMetrics.WriteMetrics
	indexCreate      *storageMetrics.WriteMetrics
	spans            *storageMetrics.WriteMetrics
	serviceOperation *storageMetrics.WriteMetrics
}

// SpanWriter is a wrapper around elastic.Client
type SpanWriter struct {
	ctx           context.Context
	client        es.Client
	logger        *zap.Logger
	writerMetrics spanWriterMetrics // TODO: build functions to wrap around each Do fn
}

// Service is the JSON struct for service:operation documents in ElasticSearch
type Service struct {
	ServiceName   string `json:"serviceName"`
	OperationName string `json:"operationName"`
}

// NewSpanWriter creates a new SpanWriter for use
func NewSpanWriter(client es.Client, logger *zap.Logger, metricsFactory metrics.Factory) *SpanWriter {
	ctx := context.Background()
	return &SpanWriter{
		ctx:    ctx,
		client: client,
		logger: logger,
		writerMetrics: spanWriterMetrics{
			exists:           storageMetrics.NewWriteMetrics(metricsFactory, "Exists"),
			indexCreate:      storageMetrics.NewWriteMetrics(metricsFactory, "IndexCreate"),
			spans:            storageMetrics.NewWriteMetrics(metricsFactory, "Spans"),
			serviceOperation: storageMetrics.NewWriteMetrics(metricsFactory, "ServiceOperation"),
		},
	}
}

// WriteSpan writes a span and its corresponding service:operation in ElasticSearch
func (s *SpanWriter) WriteSpan(span *model.Span) error {
	jaegerIndexName := spanIndexName(span)
	// Convert model.Span into json.Span
	jsonSpan := json.FromDomainEmbedProcess(span)

	if err := s.checkAndCreateIndex(jaegerIndexName, jsonSpan); err != nil {
		return err
	}
	if err := s.writeService(jaegerIndexName, jsonSpan); err != nil {
		return err
	}
	if err := s.writeSpan(jaegerIndexName, jsonSpan); err != nil {
		return err
	}
	return nil
}

func spanIndexName(span *model.Span) string {
	spanDate := span.StartTime.Format("2006-01-02")
	return "jaeger-" + spanDate
}

// Check if index exists, and create index if it does not.
func (s *SpanWriter) checkAndCreateIndex(indexName string, jsonSpan *jModel.Span) error {
	// TODO: We don't need to check every write. Try to pull this out of WriteSpan.
	start := time.Now()
	exists, err := s.client.IndexExists(indexName).Do(s.ctx)
	s.writerMetrics.exists.Emit(err, time.Since(start))
	if err != nil {
		return s.logError(jsonSpan, err, "Failed to find index", s.logger)
	}
	if !exists {
		start := time.Now()
		_, err = s.client.CreateIndex(indexName).Body(spanMapping).Do(s.ctx)
		s.writerMetrics.indexCreate.Emit(err, time.Since(start))
		if err != nil {
			return s.logError(jsonSpan, err, "Failed to create index", s.logger)
		}
	}
	return nil
}

func (s *SpanWriter) writeService(indexName string, jsonSpan *jModel.Span) error {
	// Insert serviceName:operationName document
	service := Service{
		ServiceName:   jsonSpan.Process.ServiceName,
		OperationName: jsonSpan.OperationName,
	}
	serviceID := fmt.Sprintf("%s|%s", service.ServiceName, service.OperationName)
	start := time.Now()
	_, err := s.client.Index().Index(indexName).Type(serviceType).Id(serviceID).BodyJson(service).Do(s.ctx)
	s.writerMetrics.serviceOperation.Emit(err, time.Since(start))
	if err != nil {
		return s.logError(jsonSpan, err, "Failed to insert service:operation", s.logger)
	}
	return nil
}

func (s *SpanWriter) writeSpan(indexName string, jsonSpan *jModel.Span) error {
	start := time.Now()
	_, err := s.client.Index().Index(indexName).Type(spanType).BodyJson(jsonSpan).Do(s.ctx)
	s.writerMetrics.spans.Emit(err, time.Since(start))
	if err != nil {
		return s.logError(jsonSpan, err, "Failed to insert span", s.logger)
	}
	return nil
}

func (s *SpanWriter) logError(span *jModel.Span, err error, msg string, logger *zap.Logger) error {
	logger.
		With(zap.String("trace_id", string(span.TraceID))).
		With(zap.String("span_id", string(span.SpanID))).
		With(zap.Error(err)).
		Error(msg)
	return errors.Wrap(err, msg)
}
