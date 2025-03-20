// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/internal/metrics/api"
)

func TestAllOptionSet(t *testing.T) {
	types := []processor.SpanFormat{processor.SpanFormat("sneh")}
	opts := Options.apply(
		Options.ReportBusy(true),
		Options.BlockingSubmit(true),
		Options.ExtraFormatTypes(types),
		Options.SpanFilter(func(_ *model.Span) bool { return true }),
		Options.HostMetrics(api.NullFactory),
		Options.ServiceMetrics(api.NullFactory),
		Options.Logger(zap.NewNop()),
		Options.NumWorkers(5),
		Options.PreProcessSpans(func(_ processor.Batch) {}),
		Options.Sanitizer(func(span *model.Span) *model.Span { return span }),
		Options.QueueSize(10),
		Options.DynQueueSizeWarmup(1000),
		Options.DynQueueSizeMemory(1024),
		Options.PreSave(func(_ *model.Span, _ /* tenant */ string) {}),
		Options.CollectorTags(map[string]string{"extra": "tags"}),
		Options.SpanSizeMetricsEnabled(true),
		Options.OnDroppedSpan(func(_ *model.Span) {}),
	)
	assert.EqualValues(t, 5, opts.numWorkers)
	assert.EqualValues(t, 10, opts.queueSize)
	assert.EqualValues(t, map[string]string{"extra": "tags"}, opts.collectorTags)
	assert.EqualValues(t, 1000, opts.dynQueueSizeWarmup)
	assert.EqualValues(t, 1024, opts.dynQueueSizeMemory)
	assert.True(t, opts.spanSizeMetricsEnabled)
	assert.NotNil(t, opts.onDroppedSpan)
}

func TestNoOptionsSet(t *testing.T) {
	opts := Options.apply()
	assert.EqualValues(t, flags.DefaultNumWorkers, opts.numWorkers)
	assert.EqualValues(t, 0, opts.queueSize)
	assert.Nil(t, opts.collectorTags)
	assert.False(t, opts.reportBusy)
	assert.False(t, opts.blockingSubmit)
	assert.NotPanics(t, func() { opts.preProcessSpans(processor.SpansV1{}) })
	assert.NotPanics(t, func() { opts.preSave(nil, "") })
	assert.True(t, opts.spanFilter(nil))
	span := model.Span{}
	assert.EqualValues(t, &span, opts.sanitizer(&span))
	assert.EqualValues(t, 0, opts.dynQueueSizeWarmup)
	assert.False(t, opts.spanSizeMetricsEnabled)
	assert.Nil(t, opts.onDroppedSpan)
}
