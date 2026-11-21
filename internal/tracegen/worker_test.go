// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracegen

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func Test_SimulateTraces(t *testing.T) {
	tests := []struct {
		name  string
		pause time.Duration
	}{
		{
			name:  "no pause",
			pause: 0,
		},
		{
			name:  "with pause",
			pause: time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			logger, buf := testutils.NewLogger()
			tp := sdktrace.NewTracerProvider()
			tracers := []trace.Tracer{tp.Tracer("stdout")}
			wg := sync.WaitGroup{}
			wg.Add(1)
			var running uint32 = 1
			workerID := 7
			numTraces := 7
			worker := &worker{
				logger:  logger,
				tracers: tracers,
				wg:      &wg,
				id:      workerID,
				running: &running,
				Config: Config{
					Traces:     numTraces,
					Duration:   time.Second,
					Pause:      tt.pause,
					Service:    "stdout",
					Debug:      true,
					Firehose:   true,
					ChildSpans: 1,
				},
			}
			expectedOutput := fmt.Sprintf(`{"level":"info","msg":"Worker %d generated %d traces"}`, workerID, numTraces) + "\n"
			worker.simulateTraces()
			assert.Equal(t, expectedOutput, buf.String())
		})
	}
}

func Test_SpanDuration(t *testing.T) {
	t.Run("fixed duration when min equals max", func(t *testing.T) {
		w := &worker{
			Config: Config{
				MinSpanDuration: 200 * time.Millisecond,
				MaxSpanDuration: 200 * time.Millisecond,
			},
		}
		for i := 0; i < 10; i++ {
			d := w.spanDuration()
			assert.Equal(t, 200*time.Millisecond, d)
		}
	})

	t.Run("random duration within range when min less than max", func(t *testing.T) {
		min := 100 * time.Millisecond
		max := 500 * time.Millisecond
		w := &worker{
			Config: Config{
				MinSpanDuration: min,
				MaxSpanDuration: max,
			},
		}
		for i := 0; i < 100; i++ {
			d := w.spanDuration()
			assert.GreaterOrEqual(t, d, min, "duration should be >= min")
			assert.Less(t, d, max, "duration should be < max")
		}
	})

	t.Run("default 123us backward compatible", func(t *testing.T) {
		w := &worker{
			Config: Config{
				MinSpanDuration: 123 * time.Microsecond,
				MaxSpanDuration: 123 * time.Microsecond,
			},
		}
		assert.Equal(t, 123*time.Microsecond, w.spanDuration())
	})
}
