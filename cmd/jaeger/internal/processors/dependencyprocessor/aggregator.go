// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dependencyprocessor

import (
	"context"
	"sync"
	"time"

	"github.com/apache/beam/sdks/v2/go/pkg/beam"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var beamInitOnce sync.Once

// dependencyAggregator processes spans and aggregates dependencies using Apache Beam
type dependencyAggregator struct {
	config           *Config
	telset           component.TelemetrySettings
	dependencyWriter spanstore.Writer
	inputChan        chan spanEvent
	closeChan        chan struct{}
}

// spanEvent represents a span with its service name and timestamp
type spanEvent struct {
	span        ptrace.Span
	serviceName string
	eventTime   time.Time
}

// newDependencyAggregator creates a new dependency aggregator
func newDependencyAggregator(cfg Config, telset component.TelemetrySettings, dependencyWriter spanstore.Writer) *dependencyAggregator {
	beamInitOnce.Do(func() {
		beam.Init()
	})
	return &dependencyAggregator{
		config:           &cfg,
		telset:           telset,
		dependencyWriter: dependencyWriter,
		inputChan:        make(chan spanEvent),
		closeChan:        make(chan struct{}),
	}
}

// Start begins the aggregation process
func (agg *dependencyAggregator) Start() {
	go agg.runPipeline()
}

// HandleSpan processes a single span
func (agg *dependencyAggregator) HandleSpan(ctx context.Context, span ptrace.Span, serviceName string) {
	event := spanEvent{
		span:        span,
		serviceName: serviceName,
		eventTime:   time.Now(),
	}
	select {
	case agg.inputChan <- event:
	default:
		agg.telset.Logger.Warn("Input channel full, dropping span")
	}
}

// runPipeline runs the main processing pipeline
func (agg *dependencyAggregator) runPipeline() {
	for {
		var events []spanEvent
		timer := time.NewTimer(agg.config.AggregationInterval)

	collectLoop:
		for {
			select {
			case event := <-agg.inputChan:
				events = append(events, event)
			case <-timer.C:
				break collectLoop
			case <-agg.closeChan:
				if !timer.Stop() {
					<-timer.C
				}
				if len(events) > 0 {
					agg.processEvents(context.Background(), events)
				}
				return
			}
		}

		if len(events) > 0 {
			agg.processEvents(context.Background(), events)
		}
	}
}

// processEvents processes a batch of spans using Beam pipeline
func (agg *dependencyAggregator) processEvents(ctx context.Context, events []spanEvent) {
	// Create new pipeline and scope
	p, s := beam.NewPipelineWithRoot()

	// Create initial PCollection with timestamps
	col := beam.CreateList(s, events)

	// Transform into timestamped KV pairs
	timestamped := beam.ParDo(s, func(event spanEvent) beam.WindowValue {
		return beam.WindowValue{
			Timestamp: event.eventTime,
			Windows:   beam.window.IntervalWindow{Start: event.eventTime, End: event.eventTime.Add(agg.config.InactivityTimeout)},
			Value: beam.KV{
				Key:   event.span.TraceID(),
				Value: event,
			},
		}
	}, col)

	// Apply session windows
	windowed := beam.WindowInto(s,
		beam.window.NewSessions(agg.config.InactivityTimeout),
		timestamped,
	)

	// Group by TraceID and aggregate dependencies
	grouped := beam.stats.GroupByKey(s, windowed)

	// Calculate dependencies for each trace
	dependencies := beam.ParDo(s, func(key pcommon.TraceID, iter func(*spanEvent) bool) []*model.DependencyLink {
		spanMap := make(map[pcommon.SpanID]spanEvent)
		var event *spanEvent

		// Build span map
		for iter(event) {
			spanMap[event.span.SpanID()] = *event
		}

		// Calculate dependencies
		deps := make(map[string]*model.DependencyLink)
		for _, event := range spanMap {
			parentSpanID := event.span.ParentSpanID()
			if parentEvent, hasParent := spanMap[parentSpanID]; hasParent {
				parentService := parentEvent.serviceName
				childService := event.serviceName

				// Create dependency link if services are different
				if parentService != "" && childService != "" && parentService != childService {
					depKey := parentService + "&&&" + childService
					if dep, exists := deps[depKey]; exists {
						dep.CallCount++
					} else {
						deps[depKey] = &model.DependencyLink{
							Parent:    parentService,
							Child:     childService,
							CallCount: 1,
						}
					}
				}
			}
		}

		return depMapToSlice(deps)
	}, grouped)

	// Merge results from all windows
	merged := beam.Flatten(s, dependencies)

	// Write to storage
	beam.ParDo0(s, func(deps []model.DependencyLink) {
		if err := agg.dependencyWriter.WriteDependencies(ctx, time.Now(), deps); err != nil {
			agg.telset.Logger.Error("Failed to write dependencies", zap.Error(err))
		}
	}, merged)

	// Execute pipeline
	if err := beam.Run(ctx, p); err != nil {
		agg.telset.Logger.Error("Failed to run beam pipeline", zap.Error(err))
	}
}

// Close shuts down the aggregator
func (agg *dependencyAggregator) Close() error {
	close(agg.closeChan)
	return nil
}

// depMapToSlice converts dependency map to slice
func depMapToSlice(deps map[string]*model.DependencyLink) []*model.DependencyLink {
	result := make([]*model.DependencyLink, 0, len(deps))
	for _, dep := range deps {
		result = append(result, dep)
	}
	return result
}
