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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter/common"
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
		agentTags: model.KeyValueFromMap(agentTags),
		logger:    logger,
		sanitizer: zipkin2.NewChainedSanitizer(zipkin2.StandardSanitizers...),
	}
}

// ForwardBatch sends the model.Batch to the gRPC target, implements Forwarder
func (r *Reporter) ForwardBatch(batch model.Batch) error {
	req := &api_v2.PostSpansRequest{Batch: batch}
	_, err := r.collector.PostSpans(context.Background(), req)
	if err != nil {
		return &gRPCReporterError{err}
	}
	return nil
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

func (r *Reporter) send(spans []*model.Span, process *model.Process) error {
	spans, process = common.AddProcessTags(spans, process, r.agentTags)
	batch := model.Batch{Spans: spans, Process: process}
	return r.ForwardBatch(batch)
}

// gRPCReporterError is capsulated error coming from the gRPC interface
type gRPCReporterError struct {
	Err error
}

func (g *gRPCReporterError) Error() string {
	return g.Err.Error()
}

// IsRetryable checks if the gRPC errors are temporary errors and are errors from the status package
func (g *gRPCReporterError) IsRetryable() bool {
	if state, ok := status.FromError(g.Err); ok {
		switch state.Code() {
		case codes.DeadlineExceeded:
			return true
		case codes.Unknown:
			// Sadly codes.Unknown is also returned occasionally when the collector is down, thus we must consider
			// it as retryable error.
			return true
		case codes.Unavailable:
			return true
		}
	}
	return false
}
