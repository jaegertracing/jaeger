// Copyright (c) 2019 The Jaeger Authors.
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

package grpc

import (
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	zipkin2 "github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer/zipkin"
	"github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/model"
	jConverter "github.com/jaegertracing/jaeger/model/converter/thrift/jaeger"
	"github.com/jaegertracing/jaeger/model/converter/thrift/zipkin"
	"github.com/jaegertracing/jaeger/pkg/config/tenancy"
	"github.com/jaegertracing/jaeger/pkg/multierror"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/storage"
	thrift "github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

// Reporter reports data to collector over gRPC.
type Reporter struct {
	collector api_v2.CollectorServiceClient
	agentTags []model.KeyValue
	logger    *zap.Logger
	sanitizer zipkin2.Sanitizer

	tenantHeader string
}

// NewReporter creates gRPC reporter.
func NewReporter(conn *grpc.ClientConn, agentTags map[string]string, logger *zap.Logger) *Reporter {
	return NewMultitenantReporter(conn, agentTags, "", logger)
}

func NewMultitenantReporter(conn *grpc.ClientConn, agentTags map[string]string, tenantHeader string, logger *zap.Logger) *Reporter {
	return &Reporter{
		collector:    api_v2.NewCollectorServiceClient(conn),
		agentTags:    makeModelKeyValue(agentTags),
		logger:       logger,
		sanitizer:    zipkin2.NewChainedSanitizer(zipkin2.NewStandardSanitizers()...),
		tenantHeader: tenantHeader,
	}
}

// EmitBatch implements EmitBatch() of Reporter
func (r *Reporter) EmitBatch(ctx context.Context, b *thrift.Batch) error {
	// If we aren't sending tenant headers, forward along the batch
	if r.tenantHeader == "" {
		return r.send(ctx, jConverter.ToDomain(b.Spans, nil), jConverter.ToDomainProcess(b.Process))
	}

	// Partition batch by tenant
	batches := make(map[string]*[]*thrift.Span)
	for _, span := range b.Spans {
		tenant := ""
		for _, tag := range span.Tags {
			if tag.GetKey() == app.TenancyTag {
				tenant = tag.GetVStr()
				break
			}
		}
		if tenant == "" {
			tenant = tenancy.MissingTenant
		}

		batch, ok := batches[tenant]
		if !ok {
			batch = &[]*thrift.Span{}
			batches[tenant] = batch
		}
		*batch = append(*batch, span)
	}

	// Send each tenant's spans
	var errors []error
	for tenant, partitionedBatch := range batches {
		tenantedCtx := storage.WithTenant(ctx, tenant)
		err := r.send(tenantedCtx, jConverter.ToDomain(*partitionedBatch, nil), jConverter.ToDomainProcess(b.Process))
		if err != nil {
			errors = append(errors, err)
		}
	}
	return multierror.Wrap(errors)
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
	if r.tenantHeader != "" {
		tenant := storage.GetTenant(ctx)
		md := metadata.New(map[string]string{
			r.tenantHeader: tenant,
		})
		ctx = metadata.NewOutgoingContext(ctx, md)
	}

	spans, process = addProcessTags(spans, process, r.agentTags)
	batch := model.Batch{Spans: spans, Process: process}
	req := &api_v2.PostSpansRequest{Batch: batch}
	_, err := r.collector.PostSpans(ctx, req)
	if err != nil {
		r.logger.Error("Could not send spans over gRPC", zap.Error(err))
	}
	return err
}

// addTags appends jaeger tags for the agent to every span it sends to the collector.
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
