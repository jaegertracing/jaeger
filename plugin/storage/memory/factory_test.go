// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"expvar"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/storage"
)

var _ storage.Factory = new(Factory)

func TestMemoryStorageFactory(t *testing.T) {
	f := NewFactory()
	require.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
	assert.NotNil(t, f.store)
	reader, err := f.CreateSpanReader()
	require.NoError(t, err)
	assert.Equal(t, f.store, reader)
	writer, err := f.CreateSpanWriter()
	require.NoError(t, err)
	assert.Equal(t, f.store, writer)
	depReader, err := f.CreateDependencyReader()
	require.NoError(t, err)
	assert.Equal(t, f.store, depReader)
	samplingStore, err := f.CreateSamplingStore(2)
	require.NoError(t, err)
	assert.Equal(t, 2, samplingStore.(*SamplingStore).maxBuckets)
	lock, err := f.CreateLock()
	require.NoError(t, err)
	assert.NotNil(t, lock)
}

func TestWithConfiguration(t *testing.T) {
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	command.ParseFlags([]string{"--memory.max-traces=100"})
	f.InitFromViper(v, zap.NewNop())
	assert.Equal(t, 100, f.options.Configuration.MaxTraces)
}

func TestNewFactoryWithConfig(t *testing.T) {
	cfg := Configuration{
		MaxTraces: 42,
	}
	f := NewFactoryWithConfig(cfg, metrics.NullFactory, zap.NewNop())
	assert.Equal(t, cfg, f.options.Configuration)
}

func TestPublishOpts(t *testing.T) {
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	command.ParseFlags([]string{"--memory.max-traces=100"})
	f.InitFromViper(v, zap.NewNop())

	require.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
	assert.EqualValues(t, 100, expvar.Get("jaeger_storage_memory_max_traces").(*expvar.Int).Value())
}
