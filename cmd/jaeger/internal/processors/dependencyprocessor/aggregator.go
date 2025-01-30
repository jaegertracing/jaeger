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
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var beamInitOnce sync.Once

type dependencyAggregator struct {
	config           *Config
	telset           component.TelemetrySettings
	dependencyWriter spanstore.Writer
	traces           map[pcommon.TraceID]*TraceState
	tracesLock       sync.RWMutex
	closeChan        chan struct{}
	beamPipeline     *beam.Pipeline
	beamScope        beam.Scope
}

type TraceState struct {
	spans          []*ptrace.Span
	spanMap        map[pcommon.SpanID]*ptrace.Span
	lastUpdateTime time.Time
	timer          *time.Timer
	serviceName    string
}

func newDependencyAggregator(cfg Config, telset component.TelemetrySettings, dependencyWriter spanstore.Writer) *dependencyAggregator {
	beamInitOnce.Do(func() {
		beam.Init()
	})
	p, s := beam.NewPipelineWithRoot()
	return &dependencyAggregator{
		config:           &cfg,
		telset:           telset,
		dependencyWriter: dependencyWriter,
		traces:           make(map[pcommon.TraceID]*TraceState),
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
				agg.processTraces(context.Background())
			case <-agg.closeChan:
				agg.processTraces(context.Background())
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

func (agg *dependencyAggregator) HandleSpan(ctx context.Context, span ptrace.Span, serviceName string) {
	agg.tracesLock.Lock()
	defer agg.tracesLock.Unlock()

	traceID := span.TraceID()
	spanID := span.SpanID()

	traceState, ok := agg.traces[traceID]
	if !ok {
		traceState = &TraceState{
			spans:          []*ptrace.Span{},
			spanMap:        make(map[pcommon.SpanID]*ptrace.Span),
			lastUpdateTime: time.Now(),
			serviceName:    serviceName,
		}
		agg.traces[traceID] = traceState
	}

	spanCopy := span
	traceState.spans = append(traceState.spans, &spanCopy)
	traceState.spanMap[spanID] = &spanCopy
	traceState.lastUpdateTime = time.Now()

	if traceState.timer != nil {
		traceState.timer.Stop()
	}
	traceState.timer = time.AfterFunc(agg.config.InactivityTimeout, func() {
		agg.processTraces(ctx)
	})
}

func (agg *dependencyAggregator) processTraces(ctx context.Context) {
	agg.tracesLock.Lock()
	if len(agg.traces) == 0 {
		agg.tracesLock.Unlock()
		return
	}
	traces := agg.traces
	agg.traces = make(map[pcommon.TraceID]*TraceState)
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

func (agg *dependencyAggregator) createBeamInput(traces map[pcommon.TraceID]*TraceState) beam.PCollection {
	var allSpans []*ptrace.Span
	for _, traceState := range traces {
		allSpans = append(allSpans, traceState.spans...)
	}
	if len(allSpans) == 0 {
		return beam.PCollection{}
	}
	return beam.CreateList(agg.beamScope, allSpans)
}

func (agg *dependencyAggregator) calculateDependencies(ctx context.Context, input beam.PCollection) {
	keyedSpans := beam.ParDo(agg.beamScope, func(s *ptrace.Span) (pcommon.TraceID, *ptrace.Span) {
		return s.TraceID(), s
	}, input)

	groupedSpans := beam.GroupByKey(agg.beamScope, keyedSpans)
	depLinks := beam.ParDo(agg.beamScope, func(_ pcommon.TraceID, spansIter func(*ptrace.Span) bool) []*model.DependencyLink {
		deps := map[string]*model.DependencyLink{}
		var span *ptrace.Span
		for spansIter(span) {
			processSpanOtel(deps, span, agg.traces)
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

func processSpanOtel(deps map[string]*model.DependencyLink, span *ptrace.Span, traces map[pcommon.TraceID]*TraceState) {
	traceID := span.TraceID()
	parentSpanID := span.ParentSpanID()

	traceState, ok := traces[traceID]
	if !ok {
		return
	}

	parentSpan := traceState.spanMap[parentSpanID]
	if parentSpan == nil {
		return
	}

	parentTraceState := traces[traceID]
	if parentTraceState == nil {
		return
	}

	parentService := parentTraceState.serviceName
	currentService := traceState.serviceName

	if parentService == currentService || parentService == "" || currentService == "" {
		return
	}

	depKey := parentService + "&&&" + currentService
	if _, ok := deps[depKey]; !ok {
		deps[depKey] = &model.DependencyLink{
			Parent:    parentService,
			Child:     currentService,
			CallCount: 1,
		}
	} else {
		deps[depKey].CallCount++
	}
}

// depMapToSlice converts dependency map to slice
func depMapToSlice(deps map[string]*model.DependencyLink) []*model.DependencyLink {
	retMe := make([]*model.DependencyLink, 0, len(deps))
	for _, dep := range deps {
		retMe = append(retMe, dep)
	}
	return retMe
}
