// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storageexporter

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
)

// confmap.Validate is what jaeger validate runs on loaded component configs.
func TestPartialQueueConfigUnmarshalsWithExporterhelperDefaults(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	require.False(t, cfg.QueueConfig.HasValue(), "queue is disabled when the section is absent")

	conf := confmap.NewFromStringMap(map[string]any{
		"trace_storage": "some-storage",
		"queue": map[string]any{
			"num_consumers": 10,
			"queue_size":    100_000,
			"sizer":         "items",
			"batch": map[string]any{
				"flush_timeout": "200ms",
				"min_size":      8192,
			},
		},
	})
	require.NoError(t, conf.Unmarshal(cfg))
	require.True(t, cfg.QueueConfig.HasValue())
	require.NoError(t, confmap.Validate(cfg))

	queue := cfg.QueueConfig.Get()
	assert.Equal(t, 10, queue.NumConsumers)
	assert.EqualValues(t, 100_000, queue.QueueSize)
	require.True(t, queue.Batch.HasValue())
	batch := queue.Batch.Get()
	assert.Equal(t, 200*time.Millisecond, batch.FlushTimeout)
	assert.EqualValues(t, 8192, batch.MinSize)
	assert.Positive(t, batch.Partition.IdleTimeout)
	assert.Positive(t, batch.Partition.CacheSize)
}

func TestQueueBatchSectionInheritsQueueAndPartitionDefaults(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	conf := confmap.NewFromStringMap(map[string]any{
		"trace_storage": "some-storage",
		"queue": map[string]any{
			"sizer": "items",
			"batch": map[string]any{
				"flush_timeout": "200ms",
				"min_size":      100,
			},
		},
	})
	require.NoError(t, conf.Unmarshal(cfg))
	require.NoError(t, confmap.Validate(cfg))

	queue := cfg.QueueConfig.Get()
	assert.Equal(t, 10, queue.NumConsumers)
	assert.EqualValues(t, 1_000, queue.QueueSize)
	assert.Positive(t, queue.Batch.Get().Partition.IdleTimeout)
	assert.Positive(t, queue.Batch.Get().Partition.CacheSize)
}

func TestAbsentQueueSectionLeavesQueueUnset(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	conf := confmap.NewFromStringMap(map[string]any{
		"trace_storage": "some-storage",
	})
	require.NoError(t, conf.Unmarshal(cfg))
	assert.False(t, cfg.QueueConfig.HasValue())
}
