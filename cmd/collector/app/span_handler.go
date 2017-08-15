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

package app

import (
	"github.com/uber/tchannel-go/thrift"
	"go.uber.org/zap"

	zipkinS "github.com/uber/jaeger/cmd/collector/app/sanitizer/zipkin"
	"github.com/uber/jaeger/model"
	jConv "github.com/uber/jaeger/model/converter/thrift/jaeger"
	"github.com/uber/jaeger/model/converter/thrift/zipkin"
	"github.com/uber/jaeger/thrift-gen/jaeger"
	"github.com/uber/jaeger/thrift-gen/zipkincore"
)

const (
	// JaegerFormatType is for Jaeger Spans
	JaegerFormatType = "jaeger"
	// ZipkinFormatType is for zipkin Spans
	ZipkinFormatType = "zipkin"
	// UnknownFormatType is for spans that do not have a widely defined/well-known format type
	UnknownFormatType = "unknown"
)

// ZipkinSpansHandler consumes and handles zipkin spans
type ZipkinSpansHandler interface {
	// SubmitZipkinBatch records a batch of spans in Zipkin Thrift format
	SubmitZipkinBatch(ctx thrift.Context, spans []*zipkincore.Span) ([]*zipkincore.Response, error)
}

// JaegerBatchesHandler consumes and handles Jaeger batches
type JaegerBatchesHandler interface {
	// SubmitBatches records a batch of spans in Jaeger Thrift format
	SubmitBatches(ctx thrift.Context, batches []*jaeger.Batch) ([]*jaeger.BatchSubmitResponse, error)
}

// SpanProcessor handles model spans
type SpanProcessor interface {
	// ProcessSpans processes model spans and return with either a list of true/false success or an error
	ProcessSpans(mSpans []*model.Span, spanFormat string) ([]bool, error)
}

type jaegerBatchesHandler struct {
	logger         *zap.Logger
	modelProcessor SpanProcessor
}

// NewJaegerSpanHandler returns a JaegerBatchesHandler
func NewJaegerSpanHandler(logger *zap.Logger, modelProcessor SpanProcessor) JaegerBatchesHandler {
	return &jaegerBatchesHandler{
		logger:         logger,
		modelProcessor: modelProcessor,
	}
}

func (jbh *jaegerBatchesHandler) SubmitBatches(ctx thrift.Context, batches []*jaeger.Batch) ([]*jaeger.BatchSubmitResponse, error) {
	responses := make([]*jaeger.BatchSubmitResponse, 0, len(batches))
	for _, batch := range batches {
		mSpans := make([]*model.Span, 0, len(batch.Spans))
		for _, span := range batch.Spans {
			mSpan := jConv.ToDomainSpan(span, batch.Process)
			mSpans = append(mSpans, mSpan)
		}
		oks, err := jbh.modelProcessor.ProcessSpans(mSpans, JaegerFormatType)
		if err != nil {
			return nil, err
		}
		batchOk := true
		for _, ok := range oks {
			if !ok {
				batchOk = false
				break
			}
		}
		res := &jaeger.BatchSubmitResponse{
			Ok: batchOk,
		}
		responses = append(responses, res)
	}
	return responses, nil
}

type zipkinSpanHandler struct {
	logger         *zap.Logger
	sanitizer      zipkinS.Sanitizer
	modelProcessor SpanProcessor
}

// NewZipkinSpanHandler returns a ZipkinSpansHandler
func NewZipkinSpanHandler(logger *zap.Logger, modelHandler SpanProcessor, sanitizer zipkinS.Sanitizer) ZipkinSpansHandler {
	return &zipkinSpanHandler{
		logger:         logger,
		modelProcessor: modelHandler,
		sanitizer:      sanitizer,
	}
}

// SubmitZipkinBatch records a batch of spans already in Zipkin Thrift format.
func (h *zipkinSpanHandler) SubmitZipkinBatch(ctx thrift.Context, spans []*zipkincore.Span) ([]*zipkincore.Response, error) {
	mSpans := make([]*model.Span, len(spans))
	for i, span := range spans {
		sanitized := h.sanitizer.Sanitize(span)
		mSpans[i] = ConvertZipkinToModel(sanitized, h.logger)
	}
	bools, err := h.modelProcessor.ProcessSpans(mSpans, ZipkinFormatType)
	if err != nil {
		return nil, err
	}
	responses := make([]*zipkincore.Response, len(spans))
	for i, ok := range bools {
		res := zipkincore.NewResponse()
		res.Ok = ok
		responses[i] = res
	}
	return responses, nil
}

// ConvertZipkinToModel is a helper function that logs warnings during conversion
func ConvertZipkinToModel(zSpan *zipkincore.Span, logger *zap.Logger) *model.Span {
	mSpan, err := zipkin.ToDomainSpan(zSpan)
	if err != nil {
		logger.Warn("Warning while converting zipkin to domain span", zap.Error(err))
	}
	return mSpan
}
