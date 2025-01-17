// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dependencyprocessor

import (
	"context"
	"sync"
	"time"

	"github.com/apache/beam/sdks/v2/go/pkg/beam"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/x/beamx"
	"go.opentelemetry.io/collector/component"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
)

type dependencyAggregator struct {
	config           *Config
	telset           component.TelemetrySettings
	dependencyWriter *memory.Store
	traces           map[model.TraceID]*TraceState
	tracesLock       sync.RWMutex
	closeChan        chan struct{}
	beamPipeline     *beam.Pipeline
	beamScope        beam.Scope
}

type TraceState struct {
	spans          []*model.Span
	spanMap        map[model.SpanID]*model.Span
	lastUpdateTime time.Time
	timer          *time.Timer
}

func newDependencyAggregator(cfg Config, telset component.TelemetrySettings, dependencyWriter *memory.Store) *dependencyAggregator {
	beam.Init()
	p, s := beam.NewPipelineWithRoot()
	return &dependencyAggregator{
		config:           &cfg,
		telset:           telset,
		dependencyWriter: dependencyWriter,
		traces:           make(map[model.TraceID]*TraceState),
		beamPipeline:     p,
		beamScope:        s,
	}
}

func (agg *dependencyAggregator) Start(closeChan chan struct{}) {
	agg.closeChan = closeChan
	go func() {
		ticker := time.NewTicker(agg.config.AggregationInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				agg.processTraces(context.Background()) // Pass context
			case <-agg.closeChan:
				agg.processTraces(context.Background()) // Pass context
				return
			}
		}
	}()
}

func (agg *dependencyAggregator) Close() error {
	agg.tracesLock.Lock()
	defer agg.tracesLock.Unlock()
	for _, traceState := range agg.traces {
		if traceState.timer != nil {
			traceState.timer.Stop()
		}
	}
	return nil
}

func (agg *dependencyAggregator) HandleSpan(span *model.Span) {
	agg.tracesLock.Lock()
	defer agg.tracesLock.Unlock()

	traceState, ok := agg.traces[span.TraceID]
	if !ok {
		traceState = &TraceState{
			spans:          []*model.Span{},
			spanMap:        make(map[model.SpanID]*model.Span),
			lastUpdateTime: time.Now(),
		}
		agg.traces[span.TraceID] = traceState
	}

	traceState.spans = append(traceState.spans, span)
	traceState.spanMap[span.SpanID] = span
	traceState.lastUpdateTime = time.Now()

	if traceState.timer != nil {
		traceState.timer.Stop()
	}
	traceState.timer = time.AfterFunc(agg.config.InactivityTimeout, func() {
		agg.processTraces(context.Background()) // Pass context
	})
}

func (agg *dependencyAggregator) processTraces(ctx context.Context) {
	agg.tracesLock.Lock()
	if len(agg.traces) == 0 {
		agg.tracesLock.Unlock()
		return
	}
	traces := agg.traces
	agg.traces = make(map[model.TraceID]*TraceState)
	agg.tracesLock.Unlock()
	for _, traceState := range traces {
		if traceState.timer != nil {
			traceState.timer.Stop()
		}
	}

	beamInput := agg.createBeamInput(traces)
	if beamInput.IsValid() {
		agg.calculateDependencies(ctx, beamInput)
	}
}

func (agg *dependencyAggregator) createBeamInput(traces map[model.TraceID]*TraceState) beam.PCollection {
	var allSpans []*model.Span
	for _, traceState := range traces {
		allSpans = append(allSpans, traceState.spans...)
	}
	if len(allSpans) == 0 {
		return beam.PCollection{}
	}
	return beam.CreateList(agg.beamScope, allSpans)
}

func (agg *dependencyAggregator) calculateDependencies(ctx context.Context, input beam.PCollection) {
	keyedSpans := beam.ParDo(agg.beamScope, func(s *model.Span) (model.TraceID, *model.Span) {
		return s.TraceID, s
	}, input)

	groupedSpans := beam.GroupByKey(agg.beamScope, keyedSpans)
	depLinks := beam.ParDo(agg.beamScope, func(_ model.TraceID, spansIter func(*model.Span) bool) []*model.DependencyLink {
		deps := map[string]*model.DependencyLink{}
		var span *model.Span
		for spansIter(span) {
			processSpan(deps, span, agg.traces)
		}
		return depMapToSlice(deps)
	}, groupedSpans)
	flattened := beam.Flatten(agg.beamScope, depLinks)

	beam.ParDo0(agg.beamScope, func(deps []*model.DependencyLink) {
		err := agg.dependencyWriter.WriteDependencies(ctx, time.Now(), deps)
		if err != nil {
			agg.telset.Logger.Error("Error writing dependencies", zap.Error(err))
		}
	}, flattened)

	go func() {
		err := beamx.Run(ctx, agg.beamPipeline)
		if err != nil {
			agg.telset.Logger.Error("Error running beam pipeline", zap.Error(err))
		}
		agg.beamPipeline = beam.NewPipeline()
		agg.beamScope = beam.Scope{Parent: beam.PipelineScope{ID: "dependency"}, Name: "dependency"}
	}()
}

// processSpan is a copy from the memory storage plugin
func processSpan(deps map[string]*model.DependencyLink, s *model.Span, traces map[model.TraceID]*TraceState) {
	parentSpan := seekToSpan(s, traces)
	if parentSpan != nil {
		if parentSpan.Process.ServiceName == s.Process.ServiceName {
			return
		}
		depKey := parentSpan.Process.ServiceName + "&&&" + s.Process.ServiceName
		if _, ok := deps[depKey]; !ok {
			deps[depKey] = &model.DependencyLink{
				Parent:    parentSpan.Process.ServiceName,
				Child:     s.Process.ServiceName,
				CallCount: 1,
			}
		} else {
			deps[depKey].CallCount++
		}
	}
}

func seekToSpan(span *model.Span, traces map[model.TraceID]*TraceState) *model.Span {
	traceState, ok := traces[span.TraceID]
	if !ok {
		return nil
	}
	return traceState.spanMap[span.ParentSpanID()]
}

// depMapToSlice modifies the spans to DependencyLink in the same way as the memory storage plugin
func depMapToSlice(deps map[string]*model.DependencyLink) []*model.DependencyLink {
	retMe := make([]*model.DependencyLink, 0, len(deps))
	for _, dep := range deps {
		retMe = append(retMe, dep)
	}
	return retMe
}
