package spanstore

import (
  "fmt"
  
  "github.com/jaegertracing/jaeger/model"
  //"github.com/jaegertracing/jaeger/model/converter/json"
  "github.com/uber/jaeger-lib/metrics"
  "go.uber.org/zap"
  
  jaegerClient "github.com/uber/jaeger-client-go"
)

type SpanWriter struct {
  client        jaegerClient.Reporter
  logger        *zap.Logger
}

func NewSpanWriter(
  client jaegerClient.Reporter,
  logger *zap.Logger,
  metricsFactory metrics.Factory,
) *SpanWriter {
  return &SpanWriter{
    client: client,
    logger: logger,
  }
}

// WriteSpan writes a span and its corresponding service:operation in ElasticSearch
func (s *SpanWriter) WriteSpan(span *model.Span) error {
  
  // Convert model.Span into json.Span
  //jsonSpan := json.FromDomainEmbedProcess(span)

  for _, log := range span.Logs {
    isBaggage, k, v := s.maybeGetBaggage(log)
    fmt.Println(fmt.Sprintf(" =====================> got a span log %+v, %+v, %+v", isBaggage, k, v))
  }

  // TODO: implement
  //s.logger.Info(fmt.Sprintf("%+v", jsonSpan))

  return nil
}

// Close closes SpanWriter
func (s *SpanWriter) Close() error {
  s.client.Close()
  return nil
}

func (s *SpanWriter) maybeGetBaggage(log model.Log) (bool, string, string) {
  event, key, value := "", "", ""
  for _, tag := range log.Fields {
    if tag.Key == "event" && tag.VType == model.StringType {
      event = tag.VStr
    }
    if tag.Key == "key" && tag.VType == model.StringType {
      key = tag.VStr
    }
    if tag.Key == "value" && tag.VType == model.StringType {
      value = tag.VStr
    }
  }
  return (event == "baggage" && key != "" && value != ""), key, value
}