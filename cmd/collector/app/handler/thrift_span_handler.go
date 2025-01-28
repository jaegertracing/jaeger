// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger-idl/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger-idl/thrift-gen/zipkincore"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	zipkinS "github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer/zipkin"
	jConv "github.com/jaegertracing/jaeger/model/converter/thrift/jaeger"
	"github.com/jaegertracing/jaeger/model/converter/thrift/zipkin"
)

// SubmitBatchOptions are passed to Submit methods of the handlers.
type SubmitBatchOptions struct {
	InboundTransport processor.InboundTransport
}

// ZipkinSpansHandler consumes and handles zipkin spans
type ZipkinSpansHandler interface {
	// SubmitZipkinBatch records a batch of spans in Zipkin Thrift format
	SubmitZipkinBatch(ctx context.Context, spans []*zipkincore.Span, options SubmitBatchOptions) ([]*zipkincore.Response, error)
}

// JaegerBatchesHandler consumes and handles Jaeger batches
type JaegerBatchesHandler interface {
	// SubmitBatches records a batch of spans in Jaeger Thrift format
	SubmitBatches(ctx context.Context, batches []*jaeger.Batch, options SubmitBatchOptions) ([]*jaeger.BatchSubmitResponse, error)
}

type jaegerBatchesHandler struct {
	logger         *zap.Logger
	modelProcessor processor.SpanProcessor
}

// NewJaegerSpanHandler returns a JaegerBatchesHandler
func NewJaegerSpanHandler(logger *zap.Logger, modelProcessor processor.SpanProcessor) JaegerBatchesHandler {
	return &jaegerBatchesHandler{
		logger:         logger,
		modelProcessor: modelProcessor,
	}
}

func (jbh *jaegerBatchesHandler) SubmitBatches(ctx context.Context, batches []*jaeger.Batch, options SubmitBatchOptions) ([]*jaeger.BatchSubmitResponse, error) {
	responses := make([]*jaeger.BatchSubmitResponse, 0, len(batches))
	for _, batch := range batches {
		mSpans := make([]*model.Span, 0, len(batch.Spans))
		for _, span := range batch.Spans {
			mSpan := jConv.ToDomainSpan(span, batch.Process)
			mSpans = append(mSpans, mSpan)
		}
		oks, err := jbh.modelProcessor.ProcessSpans(ctx, processor.SpansV1{
			Spans: mSpans,
			Details: processor.Details{
				InboundTransport: options.InboundTransport,
				SpanFormat:       processor.JaegerSpanFormat,
			},
		})
		if err != nil {
			jbh.logger.Error("Collector failed to process span batch", zap.Error(err))
			return nil, err
		}
		batchOk := true
		for _, ok := range oks {
			if !ok {
				batchOk = false
				break
			}
		}

		jbh.logger.Debug("Span batch processed by the collector.", zap.Bool("ok", batchOk))
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
	modelProcessor processor.SpanProcessor
}

// NewZipkinSpanHandler returns a ZipkinSpansHandler
func NewZipkinSpanHandler(logger *zap.Logger, modelHandler processor.SpanProcessor, sanitizer zipkinS.Sanitizer) ZipkinSpansHandler {
	return &zipkinSpanHandler{
		logger:         logger,
		modelProcessor: modelHandler,
		sanitizer:      sanitizer,
	}
}

// SubmitZipkinBatch records a batch of spans already in Zipkin Thrift format.
func (h *zipkinSpanHandler) SubmitZipkinBatch(ctx context.Context, spans []*zipkincore.Span, options SubmitBatchOptions) ([]*zipkincore.Response, error) {
	mSpans := make([]*model.Span, 0, len(spans))
	convCount := make([]int, len(spans))
	for i, span := range spans {
		sanitized := h.sanitizer.Sanitize(span)
		// conversion may return more than one span, e.g. when the input Zipkin span represents both client & server spans
		converted := convertZipkinToModel(sanitized, h.logger)
		convCount[i] = len(converted)
		mSpans = append(mSpans, converted...)
	}
	bools, err := h.modelProcessor.ProcessSpans(ctx, processor.SpansV1{
		Spans: mSpans,
		Details: processor.Details{
			InboundTransport: options.InboundTransport,
			SpanFormat:       processor.ZipkinSpanFormat,
		},
	})
	if err != nil {
		h.logger.Error("Collector failed to process Zipkin span batch", zap.Error(err))
		return nil, err
	}
	responses := make([]*zipkincore.Response, len(spans))
	// at this point we may have len(spans) < len(bools) if conversion results in more spans
	b := 0 // index through bools which we advance by convCount[i] for each iteration
	for i := range spans {
		res := zipkincore.NewResponse()
		res.Ok = true
		for j := 0; j < convCount[i]; j++ {
			res.Ok = res.Ok && bools[b]
			b++
		}
		responses[i] = res
	}

	h.logger.Debug(
		"Zipkin span batch processed by the collector.",
		zap.Int("received-span-count", len(spans)),
		zap.Int("processed-span-count", len(mSpans)),
	)
	return responses, nil
}

// ConvertZipkinToModel is a helper function that logs warnings during conversion
func convertZipkinToModel(zSpan *zipkincore.Span, logger *zap.Logger) []*model.Span {
	mSpans, err := zipkin.ToDomainSpan(zSpan)
	if err != nil {
		logger.Warn("Warning while converting zipkin to domain span", zap.Error(err))
	}
	return mSpans
}
