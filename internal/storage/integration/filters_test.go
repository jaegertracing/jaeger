// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
	clickhouse "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore"
	escore "github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core"
	"github.com/jaegertracing/jaeger/internal/storage/v2/memory"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestFilterCapabilities_DirectMode(t *testing.T) {
	logger, _ := testutils.NewLogger()
	telset := telemetry.NoopSettings()
	telset.Logger = logger

	f, err := memory.NewFactory(memory.Configuration{MaxTraces: 1000}, telset)
	require.NoError(t, err)
	reader, err := f.CreateTraceReader()
	require.NoError(t, err)

	s := &StorageIntegration{TraceReader: reader}
	caps := s.filterCapabilities(t)

	assert.False(t, caps.IsEmpty())
	assert.True(t, caps.SupportsLevel(expression.LevelSpan))
	assert.True(t, caps.SupportsLevel(expression.LevelResource))
	assert.True(t, caps.SupportsLevel(expression.LevelScope))
	assert.True(t, caps.SupportsLevel(expression.LevelEvent))
	assert.True(t, caps.SupportsLevel(expression.LevelLink))
	assert.True(t, caps.SupportsOperator(expression.OpEq))
	assert.True(t, caps.SupportsOperator(expression.OpSome))
}

func TestFilterCapabilities_E2EMode(t *testing.T) {
	reader := new(tracestoremocks.Reader)
	expectedCaps := tracestore.SearchCapabilities{
		Filter: &tracestore.FilterCapabilities{
			Levels:    []expression.Level{expression.LevelSpan, expression.LevelResource},
			Operators: []expression.Operator{expression.OpEq, expression.OpAnd},
		},
	}
	reader.EXPECT().SearchCapabilities(mock.Anything).Return(expectedCaps, nil).Once()

	s := &StorageIntegration{TraceReader: reader}
	caps := s.filterCapabilities(t)

	assert.False(t, caps.IsEmpty())
	assert.True(t, caps.SupportsLevel(expression.LevelSpan))
	assert.True(t, caps.SupportsLevel(expression.LevelResource))
	assert.False(t, caps.SupportsLevel(expression.LevelScope))
	assert.True(t, caps.SupportsOperator(expression.OpEq))
	assert.True(t, caps.SupportsOperator(expression.OpAnd))
	assert.False(t, caps.SupportsOperator(expression.OpSome))
}

func TestFilterCapabilities_Errors(t *testing.T) {
	t.Run("reader error", func(t *testing.T) {
		reader := new(tracestoremocks.Reader)
		reader.EXPECT().SearchCapabilities(mock.Anything).
			Return(tracestore.SearchCapabilities{}, errors.New("reader unavailable")).Once()

		s := &StorageIntegration{TraceReader: reader}
		_, err := s.getFilterCapabilities(t.Context())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reader unavailable")
	})

	t.Run("nil filter", func(t *testing.T) {
		reader := new(tracestoremocks.Reader)
		reader.EXPECT().SearchCapabilities(mock.Anything).
			Return(tracestore.SearchCapabilities{Filter: nil}, nil).Once()

		s := &StorageIntegration{TraceReader: reader}
		_, err := s.getFilterCapabilities(t.Context())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "the filter battery needs the reader's filter capabilities")
	})

	t.Run("empty filter", func(t *testing.T) {
		reader := new(tracestoremocks.Reader)
		reader.EXPECT().SearchCapabilities(mock.Anything).
			Return(tracestore.SearchCapabilities{
				Filter: &tracestore.FilterCapabilities{
					Levels:    []expression.Level{},
					Operators: []expression.Operator{},
				},
			}, nil).Once()

		s := &StorageIntegration{TraceReader: reader}
		_, err := s.getFilterCapabilities(t.Context())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "the filter battery needs the reader's filter capabilities")
	})
}

func TestFilterCapabilities_BackendCompatibility(t *testing.T) {
	t.Run("ClickHouse declarations", func(t *testing.T) {
		caps := clickhouse.FilterCapabilities()
		assert.False(t, caps.IsEmpty())
		// ClickHouse declares all five levels including Scope, so scope refusal is skipped
		assert.True(t, caps.SupportsLevel(expression.LevelScope))
		assert.True(t, caps.SupportsLevel(expression.LevelSpan))
		// ClickHouse does not declare Some, so some refusal runs
		assert.False(t, caps.SupportsOperator(expression.OpSome))
		assert.True(t, caps.SupportsOperator(expression.OpEq))
	})

	t.Run("Elasticsearch declarations", func(t *testing.T) {
		caps := escore.FilterCapabilities()
		assert.False(t, caps.IsEmpty())
		// Elasticsearch declares Span, Resource, Event (not Scope or Link), so scope refusal runs
		assert.False(t, caps.SupportsLevel(expression.LevelScope))
		assert.False(t, caps.SupportsLevel(expression.LevelLink))
		assert.True(t, caps.SupportsLevel(expression.LevelSpan))
		// Elasticsearch does not declare Some, so some refusal runs
		assert.False(t, caps.SupportsOperator(expression.OpSome))
		assert.True(t, caps.SupportsOperator(expression.OpEq))
	})
}
