package spanstore

import (
  "context"

  "github.com/pkg/errors"
  "github.com/jaegertracing/jaeger/model"
  "github.com/uber/jaeger-lib/metrics"
  "go.uber.org/zap"

  "github.com/jaegertracing/jaeger/storage/spanstore"
  storageMetrics "github.com/jaegertracing/jaeger/storage/spanstore/metrics"
  jaegerClient "github.com/uber/jaeger-client-go"
)

type SpanReader struct {
  ctx    context.Context
  client jaegerClient.Reporter
  logger *zap.Logger
}

// NewSpanReader returns a new SpanReader with a metrics.
func NewSpanReader(client jaegerClient.Reporter, logger *zap.Logger, metricsFactory metrics.Factory) spanstore.Reader {
  return storageMetrics.NewReadMetricsDecorator(newSpanReader(client, logger), metricsFactory)
}

func newSpanReader(client jaegerClient.Reporter, logger *zap.Logger) *SpanReader {
  ctx := context.Background()
  return &SpanReader{
    ctx:                     ctx,
    client:                  client,
    logger:                  logger,
  }
}

// GetTrace takes a traceID and returns a Trace associated with that traceID
func (s *SpanReader) GetTrace(traceID model.TraceID) (*model.Trace, error) {
  return nil, errors.New("Reading not supported.")
}

// FindTraces retrieves traces that match the traceQuery
func (s *SpanReader) FindTraces(traceQuery *spanstore.TraceQueryParameters) ([]*model.Trace, error) {
  return nil, errors.New("Reading not supported.")
}

// GetServices returns all services traced by Jaeger, ordered by frequency
func (s *SpanReader) GetServices() ([]string, error) {
  return nil, errors.New("Reading not supported.")
}

// GetOperations returns all operations for a specific service traced by Jaeger
func (s *SpanReader) GetOperations(service string) ([]string, error) {
  return nil, errors.New("Reading not supported.")
}