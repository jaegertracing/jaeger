// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracegen

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type worker struct {
	tracers []trace.Tracer
	running *uint32 // pointer to shared flag that indicates it's time to stop the test
	id      int     // worker id
	Config
	wg     *sync.WaitGroup // notify when done
	logger *zap.Logger

	// internal counters
	traceNo   int
	attrKeyNo int
	attrValNo int
}

const (
	fakeSpanDuration = 123 * time.Microsecond
)

func (w *worker) simulateTraces() {
	for atomic.LoadUint32(w.running) == 1 {
		svcNo := w.traceNo % len(w.tracers)
		if w.Scenario == "genai" {
			w.simulateGenAITrace(w.tracers[svcNo])
		} else {
			w.simulateOneTrace(w.tracers[svcNo])
		}
		w.traceNo++
		if w.Traces != 0 {
			if w.traceNo >= w.Traces {
				break
			}
		}
	}
	w.logger.Info(fmt.Sprintf("Worker %d generated %d traces", w.id, w.traceNo))
	w.wg.Done()
}

func (w *worker) simulateOneTrace(tracer trace.Tracer) {
	ctx := context.Background()
	attrs := []attribute.KeyValue{
		attribute.String("peer.service", "tracegen-server"),
		attribute.String("peer.host.ipv4", "1.1.1.1"),
	}
	if w.Debug {
		attrs = append(attrs, attribute.Bool("jaeger.debug", true))
	}
	if w.Firehose {
		attrs = append(attrs, attribute.Bool("jaeger.firehose", true))
	}
	start := time.Now()
	ctx, parent := tracer.Start(
		ctx,
		"lets-go",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(attrs...),
		trace.WithTimestamp(start),
	)
	w.simulateChildSpans(ctx, start, tracer)

	if w.Pause != 0 {
		parent.End()
	} else {
		totalDuration := time.Duration(w.ChildSpans) * fakeSpanDuration
		parent.End(
			trace.WithTimestamp(start.Add(totalDuration)),
		)
	}
}

func (w *worker) simulateChildSpans(ctx context.Context, start time.Time, tracer trace.Tracer) {
	for c := 0; c < w.ChildSpans; c++ {
		var attrs []attribute.KeyValue
		for a := 0; a < w.Attributes; a++ {
			key := fmt.Sprintf("attr_%02d", w.attrKeyNo)
			val := fmt.Sprintf("val_%02d", w.attrValNo)
			attrs = append(attrs, attribute.String(key, val))
			w.attrKeyNo = (w.attrKeyNo + 1) % w.AttrKeys
			w.attrValNo = (w.attrValNo + 1) % w.AttrValues
		}
		opts := []trace.SpanStartOption{
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attrs...),
		}
		childStart := start.Add(time.Duration(c) * fakeSpanDuration)
		if w.Pause == 0 {
			opts = append(opts, trace.WithTimestamp(childStart))
		}
		_, child := tracer.Start(
			ctx,
			fmt.Sprintf("child-span-%02d", c),
			opts...,
		)
		if w.Pause != 0 {
			time.Sleep(w.Pause)
			child.End()
		} else {
			child.End(
				trace.WithTimestamp(childStart.Add(fakeSpanDuration)),
			)
		}
	}
}

// simulateGenAITrace emits a multi-span trace following OpenTelemetry GenAI
// semantic conventions. The trace shape mirrors a realistic agentic workflow:
//
//	gen_ai.execute_agent (root)
//	└── gen_ai.chat      (LLM call with token usage)
//	    └── gen_ai.execute_tool  (tool invocation)
//	        └── HTTP POST /tool-api  (downstream infra span, no gen_ai.* attrs)
//	└── gen_ai.retrieve  (RAG retrieval)
//
// Token counts are varied per trace using the worker's trace counter so that
// consecutive traces produce distinct attribute values.
func (w *worker) simulateGenAITrace(tracer trace.Tracer) {
	ctx := context.Background()
	start := time.Now()

	inputTokens := 100 + (w.traceNo*37)%900
	outputTokens := 20 + (w.traceNo*13)%480

	useFakeTime := w.Pause == 0

	startSpanOpts := func(offset time.Duration, kind trace.SpanKind, attrs ...attribute.KeyValue) []trace.SpanStartOption {
		opts := []trace.SpanStartOption{
			trace.WithSpanKind(kind),
			trace.WithAttributes(attrs...),
		}
		if useFakeTime {
			opts = append(opts, trace.WithTimestamp(start.Add(offset)))
		}
		return opts
	}

	// Root: agent span
	ctx, agentSpan := tracer.Start(ctx, "gen_ai.execute_agent",
		startSpanOpts(0, trace.SpanKindServer,
			attribute.String("gen_ai.operation.name", "execute_agent"),
			attribute.String("gen_ai.system", "openai"),
			attribute.String("gen_ai.agent.name", "tracegen-agent"),
		)...,
	)

	// Child 1: LLM chat span
	chatCtx, chatSpan := tracer.Start(ctx, "gen_ai.chat",
		startSpanOpts(10*fakeSpanDuration, trace.SpanKindClient,
			attribute.String("gen_ai.operation.name", "chat"),
			attribute.String("gen_ai.system", "openai"),
			attribute.String("gen_ai.request.model", "gpt-4o"),
			attribute.Int("gen_ai.usage.input_tokens", inputTokens),
			attribute.Int("gen_ai.usage.output_tokens", outputTokens),
		)...,
	)

	// Child 1.1: tool call span
	toolCtx, toolSpan := tracer.Start(chatCtx, "gen_ai.execute_tool",
		startSpanOpts(50*fakeSpanDuration, trace.SpanKindClient,
			attribute.String("gen_ai.operation.name", "execute_tool"),
			attribute.String("gen_ai.tool.name", "get_weather"),
			attribute.String("gen_ai.tool.call.id", fmt.Sprintf("call-%d", w.traceNo)),
		)...,
	)

	// Child 1.1.1: infra HTTP span (no gen_ai.* attrs — used to test logical view filter)
	_, httpSpan := tracer.Start(toolCtx, "HTTP POST /tool-api",
		startSpanOpts(60*fakeSpanDuration, trace.SpanKindClient,
			attribute.String("http.method", "POST"),
			attribute.String("http.url", "http://tool-api/get_weather"),
			attribute.Int("http.status_code", 200),
		)...,
	)

	// Child 2: retrieval span (sibling of chat, child of agent)
	_, retrieveSpan := tracer.Start(ctx, "gen_ai.retrieve",
		startSpanOpts(420*fakeSpanDuration, trace.SpanKindClient,
			attribute.String("gen_ai.operation.name", "retrieve"),
			attribute.String("gen_ai.system", "openai"),
			attribute.String("gen_ai.retrieval.query", fmt.Sprintf("tracegen query %d", w.traceNo)),
		)...,
	)

	if w.Pause != 0 {
		time.Sleep(w.Pause)
		httpSpan.End()
		toolSpan.End()
		chatSpan.End()
		retrieveSpan.End()
		agentSpan.SetStatus(codes.Ok, "")
		agentSpan.End()
	} else {
		httpSpan.End(trace.WithTimestamp(start.Add(160 * fakeSpanDuration)))
		toolSpan.End(trace.WithTimestamp(start.Add(250 * fakeSpanDuration)))
		chatSpan.End(trace.WithTimestamp(start.Add(400 * fakeSpanDuration)))
		retrieveSpan.End(trace.WithTimestamp(start.Add(490 * fakeSpanDuration)))
		agentSpan.SetStatus(codes.Ok, "")
		agentSpan.End(trace.WithTimestamp(start.Add(500 * fakeSpanDuration)))
	}
}
