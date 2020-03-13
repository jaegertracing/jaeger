// Copyright (c) 2019 The Jaeger Authors
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

package handler

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/cmd/collector/app/handler"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

var (
	// verify API compliance
	_ jaeger.TChanCollector           = new(TChannelHandler)
	_ zipkincore.TChanZipkinCollector = new(TChannelHandler)
)

type mockZipkinHandler struct {
	spans []*zipkincore.Span
}

func (p *mockZipkinHandler) SubmitZipkinBatch(spans []*zipkincore.Span, opts handler.SubmitBatchOptions) ([]*zipkincore.Response, error) {
	p.spans = append(p.spans, spans...)
	return nil, nil
}

func TestTChannelHandler(t *testing.T) {
	jh := &mockJaegerHandler{}
	zh := &mockZipkinHandler{}
	h := NewTChannelHandler(jh, zh)
	h.SubmitBatches(nil, []*jaeger.Batch{
		{
			Spans: []*jaeger.Span{
				{OperationName: "jaeger"},
			},
		},
	})
	assert.Len(t, jh.getBatches(), 1)
	h.SubmitZipkinBatch(nil, []*zipkincore.Span{
		{
			Name: "zipkin",
		},
	})
	assert.Len(t, zh.spans, 1)
}

type mockJaegerHandler struct {
	err     error
	mux     sync.Mutex
	batches []*jaeger.Batch
}

func (p *mockJaegerHandler) SubmitBatches(batches []*jaeger.Batch, _ handler.SubmitBatchOptions) ([]*jaeger.BatchSubmitResponse, error) {
	p.mux.Lock()
	defer p.mux.Unlock()
	p.batches = append(p.batches, batches...)
	return nil, p.err
}

func (p *mockJaegerHandler) getBatches() []*jaeger.Batch {
	p.mux.Lock()
	defer p.mux.Unlock()
	return p.batches
}
