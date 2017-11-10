// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package testutils

import (
	"sync"

	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
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
