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
