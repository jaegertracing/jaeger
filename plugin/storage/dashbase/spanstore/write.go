package dashbase

import (
	"context"
	"github.com/pkg/errors"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/dashbase"
	jModel "github.com/jaegertracing/jaeger/model/json"
	storageMetrics "github.com/jaegertracing/jaeger/storage/spanstore/metrics"
	"fmt"
)

type Warnings struct {
}

type spanWriterMetrics struct {
	indexCreate *storageMetrics.WriteMetrics
	spans       *storageMetrics.WriteMetrics
}

type SpanWriter struct {
	avro          *dashbase.Avro
	ctx           context.Context
	logger        *zap.Logger
	writerMetrics spanWriterMetrics // TODO: build functions to wrap around each Do fn}
	kafkaClient   *dashbase.KafkaClient
	kafkaTopic    string
}
type Span struct {
	*jModel.Span
	StartTimeMillis uint64 `json:"startTimeMillis"`
}

func NewSpanWriter(
	kafkaClient *dashbase.KafkaClient,
	logger *zap.Logger,
	metricsFactory metrics.Factory,
	kafkaTopic string,
) *SpanWriter {
	avro := dashbase.NewAvro()
	ctx := context.Background()
	return &SpanWriter{
		avro:        avro,
		ctx:         ctx,
		logger:      logger,
		kafkaClient: kafkaClient,
		kafkaTopic:  kafkaTopic,
		writerMetrics: spanWriterMetrics{
			indexCreate: storageMetrics.NewWriteMetrics(metricsFactory, "IndexCreate"),
			spans:       storageMetrics.NewWriteMetrics(metricsFactory, "Spans"),
		},
	}
}

// WriteSpan writes a span and its corresponding service:operation in ElasticSearch
func (s *SpanWriter) WriteSpan(span *model.Span) error {
	event := SpanToDashbaseAvroEvent(span)
	message, err := s.avro.Encode(event)
	if err != nil {
		return s.logError(span, err, "Fail to encode avro", s.logger)
	}
	s.kafkaClient.Send(s.kafkaTopic, message)

	return nil
}

func (s *SpanWriter) logError(span *model.Span, err error, msg string, logger *zap.Logger) error {
	logger.
	With(zap.String("trace_id", string(span.TraceID))).
		With(zap.String("span_id", string(span.SpanID))).
		With(zap.Error(err)).
		Error(msg)
	return errors.Wrap(err, msg)
}

func SpanToDashbaseAvroEvent(span *model.Span) dashbase.Event {
	e := dashbase.Event{}
	e.TimeInMillis = span.StartTime.Unix()
	e.IdColumns["TraceID"] = span.TraceID.String()
	e.IdColumns["SpanID"] = span.SpanID.String()
	e.IdColumns["ParentSpanID"] = span.ParentSpanID.String()

	e.TextColumns["OperationName"] = span.OperationName
	e.NumberColumns["Flags"] = float64(span.Flags)
	e.NumberColumns["Duration"] = float64(span.Duration)

	for _, tag := range span.Tags {
		e.TextColumns[fmt.Sprintf("tag.%s", tag.Key)] = tag.AsString()
	}

	//todo: Log
	e.TextColumns["Process.ServiceName"] = span.Process.ServiceName
	for _, tag := range span.Process.Tags {
		e.TextColumns[fmt.Sprintf("process.%s", tag.Key)] = tag.AsString()
	}

	for i, warning := range span.Warnings {
		e.TextColumns[fmt.Sprintf("warning.%s", i)] = warning
	}

	return e

}
