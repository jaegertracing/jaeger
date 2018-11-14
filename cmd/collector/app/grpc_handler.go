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

package app

import (
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

// GRPCHandler implements gRPC CollectorService.
type GRPCHandler struct {
	logger        *zap.Logger
	spanProcessor SpanProcessor
}

// NewGRPCHandler registers routes for this handler on the given router.
func NewGRPCHandler(logger *zap.Logger, spanProcessor SpanProcessor) *GRPCHandler {
	return &GRPCHandler{
		logger:        logger,
		spanProcessor: spanProcessor,
	}
}

// PostSpans implements gRPC CollectorService.
func (g *GRPCHandler) PostSpans(stream api_v2.CollectorService_PostSpansServer) error {
	for {
		// TODO no need to close? https://stackoverflow.com/questions/46933538/how-to-close-grpc-stream-for-server
		req, err := stream.Recv()
		if err != nil {
			return err
		}
		for _, span := range req.GetBatch().Spans {
			if span.GetProcess() == nil {
				span.Process = &req.Batch.Process
			}
		}

		_, err = g.spanProcessor.ProcessSpans(req.GetBatch().Spans, JaegerFormatType)
		stream.Send(&api_v2.PostSpansResponse{})
		if err != nil {
			g.logger.Error("cannot process spans", zap.Error(err))
			// TODO does it close the stream?
			return err
		}
		stream.Send(&api_v2.PostSpansResponse{})
	}
}
