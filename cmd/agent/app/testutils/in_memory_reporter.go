package testutils

import (
	"sync"

	"github.com/uber/jaeger/thrift-gen/jaeger"
	"github.com/uber/jaeger/thrift-gen/zipkincore"
)

// InMemoryReporter collects spans in memory
type InMemoryReporter struct {
	zSpans []*zipkincore.Span
	jSpans []*jaeger.Span
	mutex  sync.Mutex
}

// NewInMemoryReporter creates new InMemoryReporter
func NewInMemoryReporter() *InMemoryReporter {
	return &InMemoryReporter{
		zSpans: make([]*zipkincore.Span, 0, 10),
		jSpans: make([]*jaeger.Span, 0, 10),
	}
}

// EmitZipkinBatch implements the corresponding method of the Reporter interface
func (i *InMemoryReporter) EmitZipkinBatch(spans []*zipkincore.Span) error {
	i.mutex.Lock()
	defer i.mutex.Unlock()
	i.zSpans = append(i.zSpans, spans...)
	return nil
}

// EmitBatch implements the corresponding method of the Reporter interface
func (i *InMemoryReporter) EmitBatch(batch *jaeger.Batch) (err error) {
	i.mutex.Lock()
	defer i.mutex.Unlock()
	i.jSpans = append(i.jSpans, batch.Spans...)
	return nil
}

// ZipkinSpans returns accumulated Zipkin spans as a copied slice
func (i *InMemoryReporter) ZipkinSpans() []*zipkincore.Span {
	i.mutex.Lock()
	defer i.mutex.Unlock()
	return i.zSpans[:]
}

// Spans returns accumulated spans as a copied slice
func (i *InMemoryReporter) Spans() []*jaeger.Span {
	i.mutex.Lock()
	defer i.mutex.Unlock()
	return i.jSpans[:]
}
