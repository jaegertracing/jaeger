// Copyright (c) 2018 The Jaeger Authors.
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
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	_ "google.golang.org/grpc/encoding/gzip" // register zip encoding
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
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
	spanOptions   processor.SpansOptions
	tenancyMgr    *tenancy.Manager
}

func newBatchConsumer(logger *zap.Logger, spanProcessor processor.SpanProcessor, transport processor.InboundTransport, spanFormat processor.SpanFormat, tenancyMgr *tenancy.Manager) batchConsumer {
	return batchConsumer{
		logger:        logger,
		spanProcessor: spanProcessor,
		spanOptions: processor.SpansOptions{
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
	_, err = c.spanProcessor.ProcessSpans(batch.Spans, processor.SpansOptions{
		InboundTransport: processor.GRPCTransport,
		SpanFormat:       processor.ProtoSpanFormat,
		Tenant:           tenant,
	})
	if err != nil {
		if err == processor.ErrBusy {
			return status.Errorf(codes.ResourceExhausted, err.Error())
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
