package elasticsearch

import (
	"encoding/json"
	"fmt"
	"github.com/olivere/elastic"

	"context"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
	elasticv7 "github.com/olivere/elastic/v7"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
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

// GetQueryJSON serializes an Elasticsearch BoolQuery and Aggregation to a JSON string.
// This is primarily for logging and debugging.
func (ql *QueryLogger) GetQueryJSON(boolQuery *elasticv7.BoolQuery, aggQuery elasticv7.Aggregation) (string, error) {
	searchSource := elasticv7.NewSearchSource().
		Query(boolQuery).
		Aggregation(aggName, aggQuery).
		Size(0)

	source, err := searchSource.Source()
	if err != nil {
		return "", fmt.Errorf("failed to get query source for logging: %w", err)
	}

	queryJSON, err := json.MarshalIndent(source, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal query to JSON for logging: %w", err)
	}
	return string(queryJSON), nil
}

// LogAndTraceQuery logs the query and adds tracing attributes.
func (ql *QueryLogger) LogAndTraceQuery(ctx context.Context, metricName, queryString string) trace.Span {
	_, span := ql.tracer.Start(ctx, metricName)
	span.SetAttributes(
		otelsemconv.DBQueryTextKey.String(queryString),
		otelsemconv.DBSystemKey.String("elasticsearch"),
		attribute.Key("component").String("es-metricsreader-query-logger"),
	)
	ql.logger.Debug("Elasticsearch metricsreader query", zap.String("query", queryString))
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
func (ql *QueryLogger) LogErrorToSpan(span trace.Span, err error) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
