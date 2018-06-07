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
	"sync"
	"time"

	pbJaeger "github.com/jaegertracing/jaeger/model/proto"
)

// Backend implements QueryServiceV1
type Backend struct {
	mu     sync.RWMutex
	traces []*pbJaeger.Trace
}

var _ pbJaeger.QueryServiceV1Server = (*Backend)(nil)

// New does new
func New() *Backend {
	return &Backend{}
}

// GetTrace gets trace
func (b *Backend) GetTrace(ctx context.Context, traceID *pbJaeger.GetTraceID) (*pbJaeger.Trace, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return &pbJaeger.Trace{
		Spans: []pbJaeger.Span{
			pbJaeger.Span{
				TraceID: pbJaeger.TraceID{Low: 123},
				// SpanID:  []byte{1, 2, 3, 4, 5, 6, 7, 8},
				// SpanID:        456,
				SpanID:        pbJaeger.NewSpanID(456),
				OperationName: "foo bar",
				StartTime:     time.Now(),
			},
		},
	}, nil
}
