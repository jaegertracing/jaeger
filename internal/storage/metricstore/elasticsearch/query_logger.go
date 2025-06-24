// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"encoding/json"

	"github.com/olivere/elastic/v7"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

// QueryLogger handles logging and tracing of Elasticsearch queries.
type QueryLogger struct {
	logger *zap.Logger
	tracer trace.Tracer
}

// NewQueryLogger creates a new QueryLogger.
func NewQueryLogger(logger *zap.Logger, tracer trace.Tracer) *QueryLogger {
	return &QueryLogger{
		logger: logger,
		tracer: tracer,
	}
}

// TraceQuery adds tracing attributes.
func (ql *QueryLogger) TraceQuery(ctx context.Context, metricName string) trace.Span {
	_, span := ql.tracer.Start(ctx, metricName)
	span.SetAttributes(
		otelsemconv.DBSystemKey.String("elasticsearch"),
		attribute.Key("component").String("es-metricsreader-query-logger"),
	)
	return span
}

// LogAndTraceResult logs the Elasticsearch query results and potentially adds them to the span.
func (ql *QueryLogger) LogAndTraceResult(span trace.Span, searchResult *elastic.SearchResult) {
	resultJSON, err := json.MarshalIndent(searchResult, "", "  ")
	if err != nil {
		ql.logger.Error("Failed to marshal search result for logging", zap.Error(err))
		// Log to span as well, but don't fail the primary operation
		span.SetAttributes(attribute.String("db.response_json.error", err.Error()))
	} else {
		ql.logger.Debug("Elasticsearch metricsreader query results", zap.String("results", string(resultJSON)))
		span.SetAttributes(attribute.String("db.response_json", string(resultJSON)))
	}
}

// LogErrorToSpan logs an error to the trace span.
func (*QueryLogger) LogErrorToSpan(span trace.Span, err error) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
