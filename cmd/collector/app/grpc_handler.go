package app

import (
	"context"
	"fmt"
	"time"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"go.uber.org/zap"
)

// GRPCHandler implements gRPC CollectorService.
type GRPCHandler struct {
	logger        *zap.Logger
	spanProcessor SpanProcessor
}

// NewGRPCHandler registers routes for this handler on the given router
func NewGRPCHandler(logger *zap.Logger, spanProcessor SpanProcessor) *GRPCHandler {
	return &GRPCHandler{
		logger:        logger,
		spanProcessor: spanProcessor,
	}
}

// PostSpans implements gRPC CollectorService.
func (g *GRPCHandler) PostSpans(ctx context.Context, r *api_v2.PostSpansRequest) (*api_v2.PostSpansResponse, error) {
	// TODO
	fmt.Printf("PostSpans(%+v)\n", *r)
	for _, s := range r.Batch.Spans {
		println(s.OperationName)
	}
	return &api_v2.PostSpansResponse{Ok: true}, nil
}

// GetTrace gets trace
func (g *GRPCHandler) GetTrace(ctx context.Context, req *api_v2.GetTraceRequest) (*api_v2.GetTraceResponse, error) {
	return &api_v2.GetTraceResponse{
		Trace: &model.Trace{
			Spans: []*model.Span{
				&model.Span{
					TraceID:       model.TraceID{Low: 123},
					SpanID:        model.NewSpanID(456),
					OperationName: "foo bar",
					StartTime:     time.Now(),
				},
			},
		},
	}, nil
}
