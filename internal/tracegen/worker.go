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
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type simulationStrategy interface {
	run(worker *worker)
}

type worker struct {
	tracers []trace.Tracer
	running *uint32
	id      int
	Config
	wg     *sync.WaitGroup
	logger *zap.Logger

	// internal counters
	traceNo      int
	attrKeyNo    int
	attrValNo    int
	spanInterval int

	strategy simulationStrategy
}

const (
	fakeSpanDuration    = 123 * time.Microsecond
	maxSpanTimeInterval = 123
)

func (w *worker) setStrategy(strategy simulationStrategy) {
	w.strategy = strategy
}

// simulateTraces main
func (w *worker) simulateTraces() {
	if w.strategy == nil {
		w.setStrategy(&randomStrategy{})
	}
	w.strategy.run(w)
	w.wg.Done()
}

// createParentSpan default common method
func (w *worker) createParentSpan(tracer trace.Tracer, start time.Time) (context.Context, trace.Span) {
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

	return tracer.Start(
		context.Background(),
		"lets-go",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(attrs...),
		trace.WithTimestamp(start),
	)
}

// generateAttributes default common method
func (w *worker) generateAttributes() []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, w.Attributes)
	for a := 0; a < w.Attributes; a++ {
		key := fmt.Sprintf("attr_%02d", w.attrKeyNo)
		val := fmt.Sprintf("val_%02d", w.attrValNo)
		attrs = append(attrs, attribute.String(key, val))
		w.attrKeyNo = (w.attrKeyNo + 1) % w.AttrKeys
		w.attrValNo = (w.attrValNo + 1) % w.AttrValues
	}
	return attrs
}

// ====================== randomStrategy ======================
type randomStrategy struct{}

func (s *randomStrategy) run(w *worker) {
	for atomic.LoadUint32(w.running) == 1 {
		svcNo := w.traceNo % len(w.tracers)
		w.simulateRandomTrace(w.tracers[svcNo])
		w.traceNo++
		if w.Traces != 0 && w.traceNo >= w.Traces {
			break
		}
	}
	w.logger.Info(fmt.Sprintf("Worker %d generated %d traces", w.id, w.traceNo))
}

func (w *worker) simulateRandomTrace(tracer trace.Tracer) {
	start := time.Now()
	ctx, parent := w.createParentSpan(tracer, start)
	w.simulateChildSpans(ctx, start, tracer)
	if w.Pause != 0 {
		parent.End()
	} else {
		totalDuration := time.Duration(w.ChildSpans) * fakeSpanDuration
		parent.End(trace.WithTimestamp(start.Add(totalDuration)))
	}
}

func (w *worker) simulateChildSpans(ctx context.Context, start time.Time, tracer trace.Tracer) {
	for c := 0; c < w.ChildSpans; c++ {
		attrs := w.generateAttributes()
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
			child.End(trace.WithTimestamp(childStart.Add(fakeSpanDuration)))
		}
	}
}

// ====================== repeatableStrategy ======================
type repeatableStrategy struct {
	startTime time.Time
}

func newRepeatableStrategy(startTime string) (*repeatableStrategy, error) {
	layout := "2006-01-02T15:04:05.000000000+00:00"
	t, err := time.Parse(layout, startTime)
	if err != nil {
		return nil, fmt.Errorf("invalid repeatable start time: %w", err)
	}
	return &repeatableStrategy{
		startTime: t.Add(time.Microsecond),
	}, nil
}

func (s *repeatableStrategy) run(w *worker) {
	w.spanInterval = 1
	w.traceNo = 0
	for atomic.LoadUint32(w.running) == 1 {
		svcNo := w.traceNo % len(w.tracers)
		w.simulateRepeatableTrace(w.tracers[svcNo], s.startTime)
		w.traceNo++
		if w.Traces != 0 && w.traceNo >= w.Traces {
			break
		}
	}
	w.logger.Info(fmt.Sprintf("Worker %d generated %d traces", w.id, w.traceNo))
}

func (w *worker) simulateRepeatableTrace(tracer trace.Tracer, baseStartTime time.Time) {
	ctx, parent := w.createParentSpan(tracer, baseStartTime)
	lastEnd := w.simulateRepeatableChildSpans(ctx, baseStartTime, tracer)
	parent.End(trace.WithTimestamp(lastEnd))
}

func (w *worker) simulateRepeatableChildSpans(ctx context.Context, baseStart time.Time, tracer trace.Tracer) time.Time {
	lastEnd := baseStart
	for c := 0; c < w.ChildSpans; c++ {
		spanStart := lastEnd
		if w.Pause != 0 {
			spanStart = spanStart.Add(w.Pause)
		}
		attrs := w.generateAttributes()
		_, child := tracer.Start(
			ctx,
			fmt.Sprintf("child-span-%02d", c),
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attrs...),
			trace.WithTimestamp(spanStart),
		)
		spanEnd := spanStart.Add(
			time.Duration(c)*time.Microsecond +
				time.Duration(w.spanInterval)*time.Microsecond,
		)
		child.End(trace.WithTimestamp(spanEnd))
		lastEnd = spanEnd
		if w.Pause != 0 {
			lastEnd = lastEnd.Add(w.Pause)
		}
		w.spanInterval++
		if w.spanInterval > maxSpanTimeInterval {
			w.spanInterval = 1
		}
	}
	return lastEnd
}
