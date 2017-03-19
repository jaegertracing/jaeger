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
