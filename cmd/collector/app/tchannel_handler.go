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

package app

import (
	"github.com/uber/tchannel-go/thrift"

	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

// TChannelHandler implements jaeger.TChanCollector and zipkincore.TChanZipkinCollector.
type TChannelHandler struct {
	jaegerHandler JaegerBatchesHandler
	zipkinHandler ZipkinSpansHandler
}

// NewTChannelHandler creates new handler that implements both Jaeger and Zipkin endpoints.
func NewTChannelHandler(
	jaegerHandler JaegerBatchesHandler,
	zipkinHandler ZipkinSpansHandler,
) *TChannelHandler {
	return &TChannelHandler{
		jaegerHandler: jaegerHandler,
		zipkinHandler: zipkinHandler,
	}
}

// SubmitZipkinBatch implements zipkincore.TChanZipkinCollector.
func (h *TChannelHandler) SubmitZipkinBatch(
	_ thrift.Context,
	spans []*zipkincore.Span,
) ([]*zipkincore.Response, error) {
	return h.zipkinHandler.SubmitZipkinBatch(spans, SubmitBatchOptions{
		InboundTransport: processor.TChannelTransport,
	})
}

// SubmitBatches implements jaeger.TChanCollector.
func (h *TChannelHandler) SubmitBatches(
	_ thrift.Context,
	batches []*jaeger.Batch,
) ([]*jaeger.BatchSubmitResponse, error) {
	return h.jaegerHandler.SubmitBatches(batches, SubmitBatchOptions{
		InboundTransport: processor.TChannelTransport,
	})
}
