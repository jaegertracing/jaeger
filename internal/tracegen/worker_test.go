// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracegen

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func Test_SimulateTraces(t *testing.T) {
	tests := []struct {
		name           string
		pause          time.Duration
		childSpans     int
		expectedOutput string
	}{
		{
			name:           "no pause",
			pause:          0,
			childSpans:     1,
			expectedOutput: `{"level":"info","msg":"Worker 7 generated 7 traces"}` + "\n",
		},
		{
			name:           "with pause",
			pause:          time.Second,
			childSpans:     1,
			expectedOutput: `{"level":"info","msg":"Worker 7 generated 7 traces"}` + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, buf := testutils.NewLogger()
			tp := sdktrace.NewTracerProvider()
			tracers := []trace.Tracer{tp.Tracer("stdout")}
			wg := sync.WaitGroup{}
			wg.Add(1)
			var running uint32 = 1
			worker := &worker{
				logger:  logger,
				tracers: tracers,
				wg:      &wg,
				id:      7,
				running: &running,
				Config: Config{
					Traces:     7,
					Duration:   time.Second,
					Pause:      tt.pause,
					Service:    "stdout",
					Debug:      true,
					Firehose:   true,
					ChildSpans: tt.childSpans,
				},
			}

			worker.simulateTraces()
			assert.Equal(t, tt.expectedOutput, buf.String())
		})
	}
}
