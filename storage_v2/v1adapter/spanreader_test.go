package v1adapter

import (
	"context"
	"testing"
	"time"

	"github.com/crossdock/crossdock-go/assert"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/iter"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	"github.com/jaegertracing/jaeger/storage_v2/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/storage_v2/tracestore/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestSpanReader_GetTrace(t *testing.T) {
	tests := []struct {
		name           string
		query          spanstore.GetTraceParameters
		expectedQuery  tracestore.GetTraceParams
		traces         []ptrace.Traces
		expectedTraces *model.Trace
		err            error
		expectedErr    error
	}{
		{
			name: "error getting trace",
			query: spanstore.GetTraceParameters{
				TraceID: model.NewTraceID(1, 2),
			},
			expectedQuery: tracestore.GetTraceParams{
				TraceID: [16]byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 2},
			},
			err:         assert.AnError,
			expectedErr: assert.AnError,
		},
		{
			name: "empty traces",
			query: spanstore.GetTraceParameters{
				TraceID: model.NewTraceID(1, 2),
			},
			expectedQuery: tracestore.GetTraceParams{
				TraceID: [16]byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 2},
			},
			traces:      []ptrace.Traces{},
			expectedErr: spanstore.ErrTraceNotFound,
		},
		{
			name: "succses",
			query: spanstore.GetTraceParameters{
				TraceID: model.NewTraceID(1, 2),
			},
			expectedQuery: tracestore.GetTraceParams{
				TraceID: [16]byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 2},
			},
			traces: func() []ptrace.Traces {
				traces := ptrace.NewTraces()
				resources := traces.ResourceSpans().AppendEmpty()
				resources.Resource().Attributes().PutStr("service.name", "service")
				scopes := resources.ScopeSpans().AppendEmpty()
				span := scopes.Spans().AppendEmpty()
				span.SetTraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 2})
				span.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 3})
				span.SetName("span")
				span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Unix(0, 0).UTC()))
				return []ptrace.Traces{traces}
			}(),
			expectedTraces: &model.Trace{
				Spans: []*model.Span{
					{
						TraceID:       model.NewTraceID(1, 2),
						SpanID:        model.NewSpanID(3),
						OperationName: "span",
						Process:       model.NewProcess("service", nil),
						StartTime:     time.Unix(0, 0).UTC(),
					},
				},
			},
		},
	}
	for _, test := range tests {
		tr := tracestoremocks.Reader{}
		tr.On("GetTraces", mock.Anything, mock.Anything).
			Return(iter.Seq2[[]ptrace.Traces, error](func(yield func([]ptrace.Traces, error) bool) {
				yield(test.traces, test.err)
			})).Once()

		sr := NewSpanReader(&tr)
		trace, err := sr.GetTrace(context.Background(), test.query)
		require.ErrorIs(t, err, test.expectedErr)
		require.Equal(t, test.expectedTraces, trace)
	}
}
