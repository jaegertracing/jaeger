// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"context"
	"errors"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	_ "google.golang.org/grpc/encoding/gzip" // register zip encoding
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger-idl/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
)

// GRPCHandler implements gRPC CollectorService.
type GRPCHandler struct {
	logger        *zap.Logger
	batchConsumer batchConsumer
}

// NewGRPCHandler registers routes for this handler on the given router.
func NewGRPCHandler(logger *zap.Logger, spanProcessor processor.SpanProcessor, tenancyMgr *tenancy.Manager) *GRPCHandler {
	return &GRPCHandler{
		logger: logger,
		batchConsumer: newBatchConsumer(logger,
			spanProcessor,
			processor.GRPCTransport,
			processor.ProtoSpanFormat,
			tenancyMgr),
	}
}

// PostSpans implements gRPC CollectorService.
func (g *GRPCHandler) PostSpans(ctx context.Context, r *api_v2.PostSpansRequest) (*api_v2.PostSpansResponse, error) {
	batch := &r.Batch
	err := g.batchConsumer.consume(ctx, batch)
	return &api_v2.PostSpansResponse{}, err
}

type batchConsumer struct {
	logger        *zap.Logger
	spanProcessor processor.SpanProcessor
	spanOptions   processor.Details // common settings for all spans
	tenancyMgr    *tenancy.Manager
}

func newBatchConsumer(logger *zap.Logger, spanProcessor processor.SpanProcessor, transport processor.InboundTransport, spanFormat processor.SpanFormat, tenancyMgr *tenancy.Manager) batchConsumer {
	return batchConsumer{
		logger:        logger,
		spanProcessor: spanProcessor,
		spanOptions: processor.Details{
			InboundTransport: transport,
			SpanFormat:       spanFormat,
		},
		tenancyMgr: tenancyMgr,
	}
}

func (c *batchConsumer) consume(ctx context.Context, batch *model.Batch) error {
	tenant, err := c.validateTenant(ctx)
	if err != nil {
		c.logger.Debug("rejecting spans (tenancy)", zap.Error(err))
		return err
	}

	for _, span := range batch.Spans {
		if span.GetProcess() == nil {
			span.Process = batch.Process
		}
	}
	_, err = c.spanProcessor.ProcessSpans(ctx, processor.SpansV1{
		Spans: batch.Spans,
		Details: processor.Details{
			InboundTransport: c.spanOptions.InboundTransport,
			SpanFormat:       c.spanOptions.SpanFormat,
			Tenant:           tenant,
		},
	})
	if err != nil {
		if errors.Is(err, processor.ErrBusy) {
			return status.Error(codes.ResourceExhausted, err.Error())
		}
		c.logger.Error("cannot process spans", zap.Error(err))
		return err
	}
	return nil
}

func (c *batchConsumer) validateTenant(ctx context.Context) (string, error) {
	if !c.tenancyMgr.Enabled {
		return "", nil
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Errorf(codes.PermissionDenied, "missing tenant header")
	}

	tenants := md.Get(c.tenancyMgr.Header)
	if len(tenants) < 1 {
		return "", status.Errorf(codes.PermissionDenied, "missing tenant header")
	} else if len(tenants) > 1 {
		return "", status.Errorf(codes.PermissionDenied, "extra tenant header")
	}

	if !c.tenancyMgr.Valid(tenants[0]) {
		return "", status.Errorf(codes.PermissionDenied, "unknown tenant")
	}

	return tenants[0], nil
}
