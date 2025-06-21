package elasticsearch

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/olivere/elastic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// test utilities
var jsonMarshal = json.Marshal

type testContext struct {
	t        *testing.T
	logger   *zap.Logger
	tp       trace.TracerProvider
	exporter *tracetest.InMemoryExporter
	tracer   trace.Tracer
	ql       *QueryLogger
}

func newTestContext(t *testing.T) *testContext {
	logger := zaptest.NewLogger(t)
	tp, exporter := tracerProvider(t)
	tracer := tp.Tracer("test")
	ql := NewQueryLogger(logger, tracer)

	return &testContext{
		t:        t,
		logger:   logger,
		tp:       tp,
		exporter: exporter,
		tracer:   tracer,
		ql:       ql,
	}
}

// tests
func TestQueryLogger(t *testing.T) {
	t.Run("NewQueryLogger", func(t *testing.T) {
		tc := newTestContext(t)
		assert.NotNil(t, tc.ql)
	})

	t.Run("TraceQuery", func(t *testing.T) {
		tc := newTestContext(t)

		span := tc.ql.TraceQuery(context.Background(), "test_query")
		assert.NotNil(t, span)

		// End the span to ensure it gets exported
		span.End()

		// Give the exporter time to process
		require.Eventually(t, func() bool {
			return len(tc.exporter.GetSpans()) > 0
		}, time.Second, 10*time.Millisecond)

		spans := tc.exporter.GetSpans()
		assert.Len(t, spans, 1)
		assert.Equal(t, "test_query", spans[0].Name)
		assert.Contains(t, spans[0].Attributes, attribute.String("db.system", "elasticsearch"))
	})

	t.Run("LogAndTraceResult", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			tc := newTestContext(t)
			_, span := tc.tracer.Start(context.Background(), "test_span")

			result := &elastic.SearchResult{TookInMillis: 10, Hits: &elastic.SearchHits{TotalHits: 5}}
			tc.ql.LogAndTraceResult(span, result)

			span.End()
			require.Eventually(t, func() bool {
				return len(tc.exporter.GetSpans()) > 0
			}, time.Second, 10*time.Millisecond)

			spans := tc.exporter.GetSpans()
			assert.Len(t, spans, 1)
		})

		t.Run("marshal error", func(t *testing.T) {
			tc := newTestContext(t)
			_, span := tc.tracer.Start(context.Background(), "test_span")

			// Mock json.Marshal to fail
			originalMarshal := jsonMarshal
			jsonMarshal = func(v interface{}) ([]byte, error) { return nil, errors.New("marshal error") }
			defer func() { jsonMarshal = originalMarshal }()

			result := &elastic.SearchResult{TookInMillis: 10}
			tc.ql.LogAndTraceResult(span, result)

			span.End()
			require.Eventually(t, func() bool {
				return len(tc.exporter.GetSpans()) > 0
			}, time.Second, 10*time.Millisecond)

			spans := tc.exporter.GetSpans()
			assert.Len(t, spans, 1)
		})
	})

	t.Run("LogErrorToSpan", func(t *testing.T) {
		tc := newTestContext(t)
		_, span := tc.tracer.Start(context.Background(), "test_span")

		testErr := errors.New("test error")
		tc.ql.LogErrorToSpan(span, testErr)

		span.End()
		require.Eventually(t, func() bool {
			return len(tc.exporter.GetSpans()) > 0
		}, time.Second, 10*time.Millisecond)

		spans := tc.exporter.GetSpans()
		assert.Len(t, spans, 1)
		assert.Equal(t, codes.Error, spans[0].Status.Code)
		assert.Equal(t, "test error", spans[0].Status.Description)
	})
}
