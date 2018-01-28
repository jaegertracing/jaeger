package spanstore

import (
  "fmt"
  "sync"
  "time"
  
  "github.com/jaegertracing/jaeger/model"
  "github.com/uber/jaeger-lib/metrics"
  "go.uber.org/zap"
  
  agentReporter "github.com/jaegertracing/jaeger/cmd/agent/app/reporter/tchannel"
  jaegerThrift "github.com/jaegertracing/jaeger/thrift-gen/jaeger"
  thriftConverter "github.com/jaegertracing/jaeger/model/converter/thrift/jaeger"
)

type SpanWriter struct {
  client             *agentReporter.Reporter
  logger             *zap.Logger
  memory             map[*model.Process][]*model.Span
  lock               *sync.Mutex
  maxBatchSize       int
  commitBatchesEvery time.Duration
  batchCommiter      *time.Ticker
}

func NewSpanWriter(
  client *agentReporter.Reporter,
  logger *zap.Logger,
  metricsFactory metrics.Factory,
  maxBatchSize int,
  commitBatchesEvery time.Duration,
) *SpanWriter {

  ticker := time.NewTicker(commitBatchesEvery)

  spanWriter := &SpanWriter{
    client:             client,
    logger:             logger,
    memory:             make(map[*model.Process][]*model.Span),
    lock:               &sync.Mutex{},
    maxBatchSize:       maxBatchSize,
    commitBatchesEvery: commitBatchesEvery,
    batchCommiter:      ticker,
  }

  go func() {
    for range ticker.C {
      spanWriter.submitAll()
    }
  }()

  return spanWriter
}

// WriteSpan writes a span and its corresponding service:operation in ElasticSearch
func (s *SpanWriter) WriteSpan(span *model.Span) error {
  
  forwardable := false
  spans := make([]*model.Span, 0)

  for _, log := range span.Logs {
    isBaggage, k, v := s.maybeGetBaggage(log)
    if isBaggage == true && k != v { // TODO: condition
      forwardable = true
      break
    }
  }

  if forwardable == true {
    
    spans = []*model.Span{ span }
    
    s.lock.Lock()
    if val, ok := s.memory[span.Process]; ok {
      spans = append(val, span)
    }
    s.lock.Unlock()

    if len(spans) < s.maxBatchSize {
      s.lock.Lock()
      s.memory[span.Process] = spans
      fmt.Println(fmt.Sprintf("Updating batch for %+v to %+v items.", &span.Process, len(s.memory[span.Process])))
      s.lock.Unlock()
    } else {
      fmt.Println(fmt.Sprintf("Immediately submitting batch of %+v items for process %+v (%+v).", len(spans), &span.Process, s.maxBatchSize))
      s.submitBatch(span.Process, spans)
    }

  }

  return nil
}

// Close closes SpanWriter
func (s *SpanWriter) Close() error {
  s.batchCommiter.Stop()
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

func (s *SpanWriter) submitAll() {
  copied := make(map[*model.Process][]*model.Span)
  s.lock.Lock()
  for k, v := range s.memory {
    copied[k] = v
  }
  s.lock.Unlock()
  for k, v := range copied {
    s.submitBatch(k, v)
  }
}

func (s *SpanWriter) submitBatch(process *model.Process, spans []*model.Span) {

  batch := &jaegerThrift.Batch{}
  batch.Process = &jaegerThrift.Process{
    process.ServiceName,
    s.convertProcessTagsToDomain(process.Tags) }
  batch.Spans = thriftConverter.FromDomain(spans)

  if err := s.client.EmitBatch(batch); err != nil {
    fmt.Println(fmt.Sprintf("Error while submitting batch of %+v items for process %+v.", len(spans), process))
  } else {
    fmt.Println(fmt.Sprintf("Batch of %+v items for process %+v submitted.", len(spans), process))
    s.lock.Lock()
    delete(s.memory, process)
    s.lock.Unlock()
  }

}

func (s *SpanWriter) convertProcessTagsToDomain(keyValues model.KeyValues) []*jaegerThrift.Tag {
  tags := make([]*jaegerThrift.Tag, 0)
  for _, kv := range keyValues {
    tags = append(tags, s.convertProcessTagToDomain(kv))
  }
  return tags
}

func (s *SpanWriter) convertProcessTagToDomain(kv model.KeyValue) *jaegerThrift.Tag {

  if kv.VType == model.StringType {
    stringValue := string(kv.VStr)
    return &jaegerThrift.Tag{
      Key:   kv.Key,
      VType: jaegerThrift.TagType_STRING,
      VStr:  &stringValue,
    }
  }

  if kv.VType == model.Int64Type {
    intValue := kv.Int64()
    return &jaegerThrift.Tag{
      Key:   kv.Key,
      VType: jaegerThrift.TagType_LONG,
      VLong: &intValue,
    }
  }

  if kv.VType == model.BinaryType {
    binaryValue := kv.Binary()
    return &jaegerThrift.Tag{
      Key:     kv.Key,
      VType:   jaegerThrift.TagType_BINARY,
      VBinary: binaryValue,
    }
  }

  if kv.VType == model.BoolType {
    boolValue := false
    if kv.VNum > 0 {
      boolValue = true
    }
    return &jaegerThrift.Tag{
      Key:   kv.Key,
      VType: jaegerThrift.TagType_BOOL,
      VBool: &boolValue,
    }
  }

  if kv.VType == model.Float64Type {
    floatValue := kv.Float64()
    return &jaegerThrift.Tag{
      Key:     kv.Key,
      VType:   jaegerThrift.TagType_DOUBLE,
      VDouble: &floatValue,
    }
  }

  errString := fmt.Sprintf("No suitable tag type found for: %#v", kv.VType)
  errTag := &jaegerThrift.Tag{
    Key:   "Error",
    VType: jaegerThrift.TagType_STRING,
    VStr:  &errString,
  }

  return errTag
}