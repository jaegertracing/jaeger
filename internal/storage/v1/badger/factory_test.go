// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package badger

import (
	"context"
	"expvar"
	"os"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	spanstoreapi "github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	badgerstore "github.com/jaegertracing/jaeger/internal/storage/v1/badger/spanstore"
)

func TestCreateSpanWriterWithJSONEncoding(t *testing.T) {
	f := NewFactory()
	f.Config.SpanEncoding = SpanEncodingJSON
	require.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
	defer func() { require.NoError(t, f.Close()) }()

	writer, err := f.CreateSpanWriter()
	require.NoError(t, err)
	span := &model.Span{
		TraceID:       model.TraceID{High: 1, Low: 2},
		SpanID:        3,
		OperationName: "operation",
		Process:       &model.Process{ServiceName: "service"},
		StartTime:     time.Now(),
		Duration:      time.Second,
	}
	require.NoError(t, writer.WriteSpan(context.Background(), span))

	var encodedAsJSON bool
	require.NoError(t, f.store.View(func(txn *badger.Txn) error {
		iterator := txn.NewIterator(badger.DefaultIteratorOptions)
		defer iterator.Close()
		for iterator.Rewind(); iterator.Valid(); iterator.Next() {
			if iterator.Item().UserMeta()&0x0F == badgerstore.EncodingJSON {
				encodedAsJSON = true
				break
			}
		}
		return nil
	}))
	assert.True(t, encodedAsJSON)

	reader, err := f.CreateSpanReader()
	require.NoError(t, err)
	trace, err := reader.GetTrace(context.Background(), spanstoreapi.GetTraceParameters{TraceID: span.TraceID})
	require.NoError(t, err)
	assert.Len(t, trace.Spans, 1)
}

func TestCreateSpanWriterRejectsUnknownEncoding(t *testing.T) {
	f := NewFactory()
	f.Config.SpanEncoding = "yaml"
	_, err := f.CreateSpanWriter()
	require.ErrorContains(t, err, "unsupported Badger span encoding")
}

func TestInitializationErrors(t *testing.T) {
	f := NewFactory()
	dir := "/root/this_should_fail" // If this test fails, you have some issues in your system
	f.Config.Ephemeral = false
	f.Config.SyncWrites = true
	f.Config.Directories.Keys = dir
	f.Config.Directories.Values = dir

	err := f.Initialize(metrics.NullFactory, zap.NewNop())
	require.Error(t, err)
}

func TestForCodecov(t *testing.T) {
	// These tests are testing our vendor packages and are intended to satisfy Codecov.
	f := NewFactory()
	err := f.Initialize(metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)

	// Get all the writers, readers, etc
	_, err = f.CreateSpanReader()
	require.NoError(t, err)

	_, err = f.CreateSpanWriter()
	require.NoError(t, err)

	_, err = f.CreateDependencyReader()
	require.NoError(t, err)

	lock, err := f.CreateLock()
	require.NoError(t, err)
	assert.NotNil(t, lock)

	// Now, remove the badger directories
	err = os.RemoveAll(f.tmpDir)
	require.NoError(t, err)

	// Now try to close, since the files have been deleted this should throw an error
	err = f.Close()
	require.Error(t, err)
}

func TestMaintenanceRun(t *testing.T) {
	// For Codecov - this does not test anything
	f := NewFactory()
	f.Config.MaintenanceInterval = 10 * time.Millisecond
	// Safeguard
	mFactory := metricstest.NewFactory(0)
	_, gs := mFactory.Snapshot()
	assert.Equal(t, int64(0), gs[lastMaintenanceRunName])
	err := f.Initialize(mFactory, zap.NewNop())
	require.NoError(t, err)
	defer f.Close()

	waiter := func(previousValue int64) int64 {
		sleeps := 0
		_, gs := mFactory.Snapshot()
		for gs[lastMaintenanceRunName] == previousValue && sleeps < 8 {
			// Wait for the scheduler
			time.Sleep(time.Duration(50) * time.Millisecond)
			sleeps++
			_, gs = mFactory.Snapshot()
		}
		assert.Greater(t, gs[lastMaintenanceRunName], previousValue)
		return gs[lastMaintenanceRunName]
	}

	runtime := waiter(0) // First run, check that it was ran and caches previous size

	// This is to for codecov only. Can break without anything else breaking as it does test badger's
	// internal implementation
	vlogSize := expvar.Get("badger_size_bytes_vlog").(*expvar.Map).Get(f.tmpDir).(*expvar.Int)
	currSize := vlogSize.Value()
	vlogSize.Set(currSize + 1<<31)

	waiter(runtime)
	_, gs = mFactory.Snapshot()
	assert.Positive(t, gs[lastValueLogCleanedName])
}

// TestMaintenanceCodecov this test is not intended to test anything, but hopefully increase coverage by triggering a log line
func TestMaintenanceCodecov(t *testing.T) {
	// For Codecov - this does not test anything
	f := NewFactory()
	f.Config.MaintenanceInterval = 10 * time.Millisecond
	mFactory := metricstest.NewFactory(0)
	err := f.Initialize(mFactory, zap.NewNop())
	require.NoError(t, err)
	defer f.Close()

	waiter := func() {
		for range 8 {
			// Wait for the scheduler
			time.Sleep(time.Duration(50) * time.Millisecond)
		}
	}

	err = f.store.Close()
	require.NoError(t, err)
	waiter() // This should trigger the logging of error
}

func TestBadgerMetrics(t *testing.T) {
	// The expvar is leaking keyparams between tests. We need to clean up a bit..
	eMap := expvar.Get("badger_size_bytes_lsm").(*expvar.Map)
	eMap.Init()

	f := NewFactory()
	f.Config.MetricsUpdateInterval = 10 * time.Millisecond
	mFactory := metricstest.NewFactory(0)
	err := f.Initialize(mFactory, zap.NewNop())
	require.NoError(t, err)
	assert.NotNil(t, f.metrics.badgerMetrics)
	_, found := f.metrics.badgerMetrics["badger_get_num_memtable"]
	assert.True(t, found)

	waiter := func(previousValue int64) int64 {
		sleeps := 0
		_, gs := mFactory.Snapshot()
		for gs["badger_get_num_memtable"] == previousValue && sleeps < 8 {
			// Wait for the scheduler
			time.Sleep(time.Duration(50) * time.Millisecond)
			sleeps++
			_, gs = mFactory.Snapshot()
		}
		assert.Equal(t, gs["badger_get_num_memtable"], previousValue)
		return gs["badger_get_num_memtable"]
	}

	vlogSize := waiter(0)
	_, gs := mFactory.Snapshot()
	assert.EqualValues(t, 0, vlogSize)
	assert.Equal(t, int64(0), gs["badger_get_num_memtable"]) // IntVal metric

	_, found = gs["badger_size_bytes_lsm"] // Map metric
	assert.True(t, found)

	require.NoError(t, f.Close())
}
