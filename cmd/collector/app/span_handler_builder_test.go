// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	cmdFlags "github.com/jaegertracing/jaeger/cmd/internal/flags"
	"github.com/jaegertracing/jaeger/internal/config"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/memory"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

func TestNewSpanHandlerBuilder(t *testing.T) {
	v, command := config.Viperize(cmdFlags.AddFlags, flags.AddFlags)

	require.NoError(t, command.ParseFlags([]string{}))
	cOpts, err := new(flags.CollectorOptions).InitFromViper(v, zap.NewNop())
	require.NoError(t, err)

	spanWriter := memory.NewStore()

	builder := &SpanHandlerBuilder{
		TraceWriter:   v1adapter.NewTraceWriter(spanWriter),
		CollectorOpts: cOpts,
		TenancyMgr:    &tenancy.Manager{},
	}
	assert.NotNil(t, builder.logger())
	assert.NotNil(t, builder.metricsFactory())

	builder = &SpanHandlerBuilder{
		TraceWriter:    v1adapter.NewTraceWriter(spanWriter),
		CollectorOpts:  cOpts,
		Logger:         zap.NewNop(),
		MetricsFactory: metrics.NullFactory,
		TenancyMgr:     &tenancy.Manager{},
	}

	spanProcessor, err := builder.BuildSpanProcessor()
	require.NoError(t, err)
	spanHandlers := builder.BuildHandlers(spanProcessor)
	assert.NotNil(t, spanHandlers.ZipkinSpansHandler)
	assert.NotNil(t, spanHandlers.JaegerBatchesHandler)
	assert.NotNil(t, spanHandlers.GRPCHandler)
	assert.NotNil(t, spanProcessor)
	require.NoError(t, spanProcessor.Close())
}

func TestDefaultSpanFilter(t *testing.T) {
	assert.True(t, defaultSpanFilter(nil))
}

func TestNegativeDurationSpanSanitizer(t *testing.T) {
	builder := &SpanHandlerBuilder{
		Logger: zap.NewNop(),
	}

	spans := []*model.Span{
		{
			TraceID:  model.NewTraceID(0, 1),
			SpanID:   model.NewSpanID(1),
			Duration: time.Duration(0),
			Tags:     []model.KeyValue{},
		},
		{
			TraceID:  model.NewTraceID(0, 2),
			SpanID:   model.NewSpanID(2),
			Duration: time.Duration(10000),
			Tags:     []model.KeyValue{},
		},
		{
			TraceID:  model.NewTraceID(0, 3),
			SpanID:   model.NewSpanID(3),
			Duration: time.Duration(-5000),
			Tags:     []model.KeyValue{},
		},
		{
			TraceID:  model.NewTraceID(0, 4),
			SpanID:   model.NewSpanID(4),
			Duration: time.Duration(-1),
			Tags:     []model.KeyValue{},
		},
	}

	for i, span := range spans {
		result := builder.NegativeDurationSpanSanitizer(span)

		switch i {
		case 0:
			assert.Equal(t, time.Duration(0), result.Duration)
			assert.Empty(t, result.Warnings)
			assert.Empty(t, result.Tags)
		case 1:
			assert.Equal(t, time.Duration(10000), result.Duration)
			assert.Empty(t, result.Warnings)
			assert.Empty(t, result.Tags)
		case 2:
			assert.Equal(t, time.Duration(1), result.Duration)
			assert.Len(t, result.Warnings, 1)
			assert.Contains(t, result.Warnings[0], "Negative duration")
			assert.Len(t, result.Tags, 1)
			assert.Equal(t, "duration-adjusted", result.Tags[0].Key)
			assert.Equal(t, int64(1), result.Tags[0].VInt64)
		case 3:
			assert.Equal(t, time.Duration(1), result.Duration)
			assert.Len(t, result.Warnings, 1)
			assert.Contains(t, result.Warnings[0], "Negative duration")
			assert.Len(t, result.Tags, 1)
			assert.Equal(t, "duration-adjusted", result.Tags[0].Key)
			assert.Equal(t, int64(1), result.Tags[0].VInt64)
		}
	}
}
