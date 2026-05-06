// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracegen

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func Test_SimulateGenAITrace(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	tracers := []trace.Tracer{tp.Tracer("genai-test")}

	wg := sync.WaitGroup{}
	wg.Add(1)
	var running uint32 = 1
	w := &worker{
		logger:  zap.NewNop(),
		tracers: tracers,
		wg:      &wg,
		id:      0,
		running: &running,
		Config: Config{
			Traces:   1,
			Pause:    0,
			Service:  "tracegen",
			Scenario: "genai",
		},
	}
	w.simulateTraces()

	spans := exporter.GetSpans()
	require.Len(t, spans, 5, "expected 5 spans: agent, chat, execute_tool, http, retrieve")

	spanNames := make(map[string]tracetest.SpanStub)
	for _, s := range spans {
		spanNames[s.Name] = s
	}

	assert.Contains(t, spanNames, "gen_ai.execute_agent")
	assert.Contains(t, spanNames, "gen_ai.chat")
	assert.Contains(t, spanNames, "gen_ai.execute_tool")
	assert.Contains(t, spanNames, "HTTP POST /tool-api")
	assert.Contains(t, spanNames, "gen_ai.retrieve")

	// Verify gen_ai.* attributes are present on the LLM chat span.
	chatSpan := spanNames["gen_ai.chat"]
	attrMap := make(map[string]string)
	for _, kv := range chatSpan.Attributes {
		attrMap[string(kv.Key)] = kv.Value.AsString()
	}
	assert.Equal(t, "chat", attrMap["gen_ai.operation.name"])
	assert.Equal(t, "openai", attrMap["gen_ai.system"])
	assert.Equal(t, "gpt-4o", attrMap["gen_ai.request.model"])

	// Verify the infra span carries no gen_ai.* attributes.
	httpSpan := spanNames["HTTP POST /tool-api"]
	for _, kv := range httpSpan.Attributes {
		assert.NotContains(t, string(kv.Key), "gen_ai.",
			"infra span must not carry gen_ai.* attributes")
	}
}

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
			pause: time.Second,
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
					ChildSpans: 1,
				},
			}
			expectedOutput := `{"level":"info","msg":"Worker 7 generated 7 traces"}` + "\n"
			worker.simulateTraces()
			assert.Equal(t, expectedOutput, buf.String())
		})
	}
}
