// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/badger"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

// openBadgerFactory opens a persistent Badger factory at dir and runs the test.
// The factory is closed after the test completes.
func openBadgerFactory(t *testing.T, dir string, test func(t *testing.T, sw spanstore.Writer, sr spanstore.Reader)) {
	f := badger.NewFactory()
	f.Config.Directories.Keys = dir
	f.Config.Directories.Values = dir
	f.Config.Ephemeral = false
	f.Config.SyncWrites = true
	err := f.Initialize(metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)
	defer func() {
		require.NoError(t, f.Close())
	}()

	sw, err := f.CreateSpanWriter()
	require.NoError(t, err)
	sr, err := f.CreateSpanReader()
	require.NoError(t, err)

	test(t, sw, sr)
}

// TestGetOperationsWithSpanKind verifies that GetOperations filters by spanKind
// and returns populated SpanKind fields when spans have span.kind tags.
func TestGetOperationsWithSpanKind(t *testing.T) {
	runFactoryTest(t, func(_ testing.TB, sw spanstore.Writer, sr spanstore.Reader) {
		spans := []model.Span{
			{
				TraceID:       model.TraceID{Low: 1, High: 1},
				SpanID:        model.SpanID(1),
				OperationName: "get-users",
				Process:       &model.Process{ServiceName: "api-gateway"},
				Tags:          model.KeyValues{model.String("span.kind", "server")},
				StartTime:     time.Now(),
				Duration:      time.Millisecond,
			},
			{
				TraceID:       model.TraceID{Low: 1, High: 1},
				SpanID:        model.SpanID(2),
				OperationName: "db-query",
				Process:       &model.Process{ServiceName: "api-gateway"},
				Tags:          model.KeyValues{model.String("span.kind", "client")},
				StartTime:     time.Now(),
				Duration:      time.Millisecond,
			},
			{
				TraceID:       model.TraceID{Low: 2, High: 1},
				SpanID:        model.SpanID(3),
				OperationName: "get-users",
				Process:       &model.Process{ServiceName: "api-gateway"},
				Tags:          model.KeyValues{model.String("span.kind", "client")},
				StartTime:     time.Now(),
				Duration:      time.Millisecond,
			},
		}
		for i := range spans {
			require.NoError(t, sw.WriteSpan(context.Background(), &spans[i]))
		}

		// Empty spanKind returns all
		ops, err := sr.GetOperations(context.Background(), tracestore.OperationQueryParams{
			ServiceName: "api-gateway",
		})
		require.NoError(t, err)
		assert.ElementsMatch(t, []tracestore.Operation{
			{Name: "get-users", SpanKind: "server"},
			{Name: "db-query", SpanKind: "client"},
			{Name: "get-users", SpanKind: "client"},
		}, ops)

		// Filter server
		ops, err = sr.GetOperations(context.Background(), tracestore.OperationQueryParams{
			ServiceName: "api-gateway",
			SpanKind:    "server",
		})
		require.NoError(t, err)
		assert.Equal(t, []tracestore.Operation{
			{Name: "get-users", SpanKind: "server"},
		}, ops)

		// Filter client
		ops, err = sr.GetOperations(context.Background(), tracestore.OperationQueryParams{
			ServiceName: "api-gateway",
			SpanKind:    "client",
		})
		require.NoError(t, err)
		assert.ElementsMatch(t, []tracestore.Operation{
			{Name: "db-query", SpanKind: "client"},
			{Name: "get-users", SpanKind: "client"},
		}, ops)

		// Filter with no matches
		ops, err = sr.GetOperations(context.Background(), tracestore.OperationQueryParams{
			ServiceName: "api-gateway",
			SpanKind:    "producer",
		})
		require.NoError(t, err)
		assert.Empty(t, ops)
	})
}

// TestGetOperationsWithoutSpanKind verifies backward compatibility: spans without
// span.kind tags appear with empty SpanKind and are excluded when filtering by kind.
func TestGetOperationsWithoutSpanKind(t *testing.T) {
	runFactoryTest(t, func(_ testing.TB, sw spanstore.Writer, sr spanstore.Reader) {
		s := model.Span{
			TraceID:       model.TraceID{Low: 1, High: 1},
			SpanID:        model.SpanID(1),
			OperationName: "legacy-op",
			Process:       &model.Process{ServiceName: "old-service"},
			StartTime:     time.Now(),
			Duration:      time.Millisecond,
		}
		require.NoError(t, sw.WriteSpan(context.Background(), &s))

		// Empty spanKind returns the entry with empty SpanKind
		ops, err := sr.GetOperations(context.Background(), tracestore.OperationQueryParams{
			ServiceName: "old-service",
		})
		require.NoError(t, err)
		assert.Equal(t, []tracestore.Operation{
			{Name: "legacy-op", SpanKind: ""},
		}, ops)

		// Filter by specific kind excludes the entry
		ops, err = sr.GetOperations(context.Background(), tracestore.OperationQueryParams{
			ServiceName: "old-service",
			SpanKind:    "server",
		})
		require.NoError(t, err)
		assert.Empty(t, ops)
	})
}

// TestGetOperationsSpanKindPersistence verifies that 0x85 index keys survive
// DB close/reopen and preload correctly into the cache with spanKind.
func TestGetOperationsSpanKindPersistence(t *testing.T) {
	dir := t.TempDir()

	// Phase 1: write spans with spanKind, then close
	openBadgerFactory(t, dir, func(t *testing.T, sw spanstore.Writer, _ spanstore.Reader) {
		spans := []model.Span{
			{
				TraceID:       model.TraceID{Low: 1, High: 1},
				SpanID:        model.SpanID(1),
				OperationName: "get-users",
				Process:       &model.Process{ServiceName: "api-gateway"},
				Tags:          model.KeyValues{model.String("span.kind", "server")},
				StartTime:     time.Now(),
				Duration:      time.Millisecond,
			},
			{
				TraceID:       model.TraceID{Low: 1, High: 1},
				SpanID:        model.SpanID(2),
				OperationName: "db-query",
				Process:       &model.Process{ServiceName: "api-gateway"},
				Tags:          model.KeyValues{model.String("span.kind", "client")},
				StartTime:     time.Now(),
				Duration:      time.Millisecond,
			},
		}
		for i := range spans {
			require.NoError(t, sw.WriteSpan(context.Background(), &spans[i]))
		}
	})

	// Phase 2: reopen, verify spanKind preloaded from 0x85 keys
	openBadgerFactory(t, dir, func(t *testing.T, _ spanstore.Writer, sr spanstore.Reader) {
		ops, err := sr.GetOperations(context.Background(), tracestore.OperationQueryParams{
			ServiceName: "api-gateway",
			SpanKind:    "server",
		})
		require.NoError(t, err)
		assert.Equal(t, []tracestore.Operation{
			{Name: "get-users", SpanKind: "server"},
		}, ops)

		ops, err = sr.GetOperations(context.Background(), tracestore.OperationQueryParams{
			ServiceName: "api-gateway",
			SpanKind:    "client",
		})
		require.NoError(t, err)
		assert.Equal(t, []tracestore.Operation{
			{Name: "db-query", SpanKind: "client"},
		}, ops)
	})
}

// TestGetOperationsSpanKindCaseHandling verifies that uppercase span.kind
// values are not recognized at write time and treated as unspecified/empty
// in GetOperations results.
func TestGetOperationsSpanKindCaseHandling(t *testing.T) {
	dir := t.TempDir()

	openBadgerFactory(t, dir, func(t *testing.T, sw spanstore.Writer, _ spanstore.Reader) {
		s := model.Span{
			TraceID:       model.TraceID{Low: 1, High: 1},
			SpanID:        model.SpanID(1),
			OperationName: "op",
			Process:       &model.Process{ServiceName: "svc"},
			Tags:          model.KeyValues{model.String("span.kind", "SERVER")},
			StartTime:     time.Now(),
			Duration:      time.Millisecond,
		}
		require.NoError(t, sw.WriteSpan(context.Background(), &s))
	})

	// Reopen forces preload from 0x85 keys on disk
	openBadgerFactory(t, dir, func(t *testing.T, _ spanstore.Writer, sr spanstore.Reader) {
		ops, err := sr.GetOperations(context.Background(), tracestore.OperationQueryParams{
			ServiceName: "svc",
		})
		require.NoError(t, err)
		assert.Equal(t, []tracestore.Operation{
			{Name: "op", SpanKind: ""},
		}, ops)
	})
}
