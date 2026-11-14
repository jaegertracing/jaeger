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
	assert.Equal(t, 90*time.Second, batch.Partition.IdleTimeout)
	assert.Equal(t, 10000, batch.Partition.CacheSize)
	assert.Empty(t, batch.Partition.MetadataKeys)
}

func TestQueueBatchSectionInheritsQueueAndPartitionDefaults(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	conf := confmap.NewFromStringMap(map[string]any{
		"trace_storage": "some-storage",
		"queue": map[string]any{
			"sizer": "items",
			"batch": map[string]any{
				"flush_timeout": "200ms",
				// The inherited queue_size is 1000, and min_size must be at most that.
				// The exporterhelper batch default of 8192 would fail the check.
				"min_size": 100,
			},
		},
	})
	require.NoError(t, conf.Unmarshal(cfg))
	require.NoError(t, confmap.Validate(cfg))

	queue := cfg.QueueConfig.Get()
	assert.Equal(t, 10, queue.NumConsumers)
	assert.EqualValues(t, 1_000, queue.QueueSize)
	partition := queue.Batch.Get().Partition
	assert.Equal(t, 90*time.Second, partition.IdleTimeout)
	assert.Equal(t, 10000, partition.CacheSize)
	assert.Empty(t, partition.MetadataKeys)
}

func TestQueueWithoutBatchLeavesBatchDisabled(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	conf := confmap.NewFromStringMap(map[string]any{
		"trace_storage": "some-storage",
		"queue": map[string]any{
			"num_consumers": 10,
			"queue_size":    1000,
		},
	})
	require.NoError(t, conf.Unmarshal(cfg))
	require.NoError(t, confmap.Validate(cfg))

	queue := cfg.QueueConfig.Get()
	assert.False(t, queue.Batch.HasValue())
	assert.Equal(t, 10, queue.NumConsumers)
	assert.EqualValues(t, 1_000, queue.QueueSize)
}

func TestAbsentQueueSectionLeavesQueueUnset(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	conf := confmap.NewFromStringMap(map[string]any{
		"trace_storage": "some-storage",
	})
	require.NoError(t, conf.Unmarshal(cfg))
	assert.False(t, cfg.QueueConfig.HasValue())
}
