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
		sanitizer: zipkin2.NewChainedSanitizer(zipkin2.StandardSanitizers...),
	}
}

// EmitBatch implements EmitBatch() of Reporter
func (r *Reporter) EmitBatch(b *thrift.Batch) error {
	return r.send(jConverter.ToDomain(b.Spans, nil), jConverter.ToDomainProcess(b.Process))
}

// EmitZipkinBatch implements EmitZipkinBatch() of Reporter
func (r *Reporter) EmitZipkinBatch(zSpans []*zipkincore.Span) error {
	for i := range zSpans {
		zSpans[i] = r.sanitizer.Sanitize(zSpans[i])
	}
	trace, err := zipkin.ToDomain(zSpans)
	if err != nil {
		return err
	}
	return r.send(trace.Spans, nil)
}

// Close the reporter
func (r *Reporter) Close() error {
	return nil
}

func (r *Reporter) send(spans []*model.Span, process *model.Process) error {
	spans, process = addProcessTags(spans, process, r.agentTags)
	batch := model.Batch{Spans: spans, Process: process}
	req := &api_v2.PostSpansRequest{Batch: batch}
	_, err := r.collector.PostSpans(context.Background(), req)
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
