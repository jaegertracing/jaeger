// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dependencyprocessor

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

type dependencyProcessor struct {
	config           *Config
	aggregator       *dependencyAggregator // Define the aggregator below.
	telset           component.TelemetrySettings
	dependencyWriter spanstore.Writer
	closeChan        chan struct{}
	nextConsumer     consumer.Traces
}

func newDependencyProcessor(cfg Config, telset component.TelemetrySettings, nextConsumer consumer.Traces) *dependencyProcessor {
	return &dependencyProcessor{
		config:       &cfg,
		telset:       telset,
		closeChan:    make(chan struct{}),
		nextConsumer: nextConsumer,
	}
}

func (tp *dependencyProcessor) Start(_ context.Context, host component.Host) error {
	storageName := tp.config.StorageName
	if storageName == "" {
		storageName = "memory"
	}

	f, err := jaegerstorage.GetStorageFactory(storageName, host)
	if err != nil {
		return fmt.Errorf("failed to get storage factory: %w", err)
	}

	writer, err := f.CreateSpanWriter()
	if err != nil {
		return fmt.Errorf("failed to create dependency writer: %w", err)
	}

	tp.dependencyWriter = writer

	tp.aggregator = newDependencyAggregator(*tp.config, tp.telset, tp.dependencyWriter)
	tp.aggregator.Start(tp.closeChan)
	return nil
}

// Shutdown implements processor.Traces
func (tp *dependencyProcessor) Shutdown(ctx context.Context) error {
	if tp.aggregator != nil {
		if err := tp.aggregator.Close(); err != nil {
			return fmt.Errorf("failed to stop the dependency aggregator : %w", err)
		}
	}
	return nil
}

// Capabilities implements processor.Traces
func (p *dependencyProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

func (dp *dependencyProcessor) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	for i := 0; i < td.ResourceSpans().Len(); i++ {
		rs := td.ResourceSpans().At(i)
		serviceName := getServiceName(rs.Resource())

		for j := 0; j < rs.ScopeSpans().Len(); j++ {
			ss := rs.ScopeSpans().At(j)
			for k := 0; k < ss.Spans().Len(); k++ {
				span := ss.Spans().At(k)
				dp.aggregator.HandleSpan(ctx, span, serviceName)
			}
		}
	}

	return dp.nextConsumer.ConsumeTraces(ctx, td)
}

func getServiceName(resource pcommon.Resource) string {
	serviceNameAttr, found := resource.Attributes().Get("service.name")
	if !found {
		return ""
	}
	return serviceNameAttr.Str()
}
