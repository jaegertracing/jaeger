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

package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

// Backend implements QueryServiceV1
type Backend struct {
	mu     sync.RWMutex
	traces []*model.Trace
}

var _ api_v2.QueryServiceServer = (*Backend)(nil)

// New does new
func New() *Backend {
	return &Backend{}
}

// PostSpans implements gRPC CollectorService.
func (b *Backend) PostSpans(ctx context.Context, r *api_v2.PostSpansRequest) (*api_v2.PostSpansResponse, error) {
	fmt.Printf("PostSpans(%+v)\n", *r)
	for _, s := range r.Batch.Spans {
		println(s.OperationName)
	}
	return &api_v2.PostSpansResponse{Ok: true}, nil
}

// GetTrace gets trace
func (b *Backend) GetTrace(ctx context.Context, req *api_v2.GetTraceRequest) (*api_v2.GetTraceResponse, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

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
