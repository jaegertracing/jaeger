// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	zipkin2 "github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer/zipkin"
	"github.com/jaegertracing/jaeger/model"
	jConverter "github.com/jaegertracing/jaeger/model/converter/thrift/jaeger"
	"github.com/jaegertracing/jaeger/model/converter/thrift/zipkin"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	thrift "github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

// Reporter reports data to collector over gRPC.
type Reporter struct {
	collector api_v2.CollectorServiceClient
	agentTags []model.KeyValue
	logger    *zap.Logger
	sanitizer zipkin2.Sanitizer
}

// NewReporter creates gRPC reporter.
func NewReporter(conn *grpc.ClientConn, agentTags map[string]string, logger *zap.Logger) *Reporter {
	return &Reporter{
		collector: api_v2.NewCollectorServiceClient(conn),
		agentTags: makeModelKeyValue(agentTags),
		logger:    logger,
		sanitizer: zipkin2.NewChainedSanitizer(zipkin2.NewStandardSanitizers()...),
	}
}

// EmitBatch implements EmitBatch() of Reporter
func (r *Reporter) EmitBatch(ctx context.Context, b *thrift.Batch) error {
	return r.send(ctx, jConverter.ToDomain(b.Spans, nil), jConverter.ToDomainProcess(b.Process))
}

// EmitZipkinBatch implements EmitZipkinBatch() of Reporter
func (r *Reporter) EmitZipkinBatch(ctx context.Context, zSpans []*zipkincore.Span) error {
	for i := range zSpans {
		zSpans[i] = r.sanitizer.Sanitize(zSpans[i])
	}
	trace, err := zipkin.ToDomain(zSpans)
	if err != nil {
		return err
	}
	return r.send(ctx, trace.Spans, nil)
}

func (r *Reporter) send(ctx context.Context, spans []*model.Span, process *model.Process) error {
	spans, process = addProcessTags(spans, process, r.agentTags)
	batch := model.Batch{Spans: spans, Process: process}
	req := &api_v2.PostSpansRequest{Batch: batch}
	_, err := r.collector.PostSpans(ctx, req)
	if err != nil {
		stat, ok := status.FromError(err)
		if ok && stat.Code() == codes.PermissionDenied && stat.Message() == "missing tenant header" {
			r.logger.Debug("Could not report untenanted spans over gRPC", zap.Error(err))
		} else {
			r.logger.Error("Could not send spans over gRPC", zap.Error(err))
		}
		err = fmt.Errorf("failed to export spans: %w", err)
	}
	return err
}

// addProcessTags appends jaeger tags for the agent to every span it sends to the collector.
func addProcessTags(spans []*model.Span, process *model.Process, agentTags []model.KeyValue) ([]*model.Span, *model.Process) {
	if len(agentTags) == 0 {
		return spans, process
	}
	if process != nil {
		process.Tags = append(process.Tags, agentTags...)
	}
	for _, span := range spans {
		if span.Process != nil {
			span.Process.Tags = append(span.Process.Tags, agentTags...)
		}
	}
	return spans, process
}

func makeModelKeyValue(agentTags map[string]string) []model.KeyValue {
	tags := make([]model.KeyValue, 0, len(agentTags))
	for k, v := range agentTags {
		tag := model.String(k, v)
		tags = append(tags, tag)
	}

	return tags
}
