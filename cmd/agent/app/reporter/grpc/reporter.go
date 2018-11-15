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
	logger    *zap.Logger
}

// NewReporter creates gRPC reporter.
func NewReporter(conn *grpc.ClientConn, logger *zap.Logger) *Reporter {
	return &Reporter{
		collector: api_v2.NewCollectorServiceClient(conn),
		logger:    logger,
	}
}

// EmitBatch implements EmitBatch() of Reporter
func (r *Reporter) EmitBatch(b *thrift.Batch) error {
	return r.send(jConverter.ToDomain(b.Spans, nil), *jConverter.ToDomainProcess(b.Process))
}

// EmitZipkinBatch implements EmitZipkinBatch() of Reporter
func (r *Reporter) EmitZipkinBatch(zSpans []*zipkincore.Span) error {
	trace, err := zipkin.ToDomain(zSpans)
	if err != nil {
		return err
	}
	var process model.Process
	if spans := trace.GetSpans(); len(spans) > 0 {
		process = *spans[0].Process
	}
	return r.send(trace.Spans, process)
}

func (r *Reporter) send(spans []*model.Span, process model.Process) error {
	batch := model.Batch{Spans: spans, Process: process}
	req := &api_v2.PostSpansRequest{Batch: batch}
	_, err := r.collector.PostSpans(context.Background(), req)
	if err != nil {
		r.logger.Error("Could not send spans over gRPC", zap.Error(err), zap.String("service", batch.Process.ServiceName))
	}
	return err
}
