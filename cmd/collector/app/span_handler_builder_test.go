// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/flags"
	cmdFlags "github.com/jaegertracing/jaeger/cmd/internal/flags"
	"github.com/jaegertracing/jaeger/internal/storage/v1/memory"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/v1adapter"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
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
