// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

func TestBuildRotation(t *testing.T) {
	logger := zap.NewNop()
	ts := time.Date(2024, time.March, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		prefix         string
		rc             config.RotationConfig
		remoteClusters []string
		wantWrite      string
		wantRead       []string
	}{
		{
			name:   "periodic rotation",
			prefix: "jaeger-span-",
			rc: config.RotationConfig{
				Periodic: configoptional.Some(config.PeriodicRotation{
					DateLayout:        "2006-01-02",
					RolloverFrequency: "day",
				}),
			},
			wantWrite: "jaeger-span-2024-03-15",
			wantRead:  []string{"jaeger-span-2024-03-15"},
		},
		{
			name:   "manual rollover with explicit aliases",
			prefix: "jaeger-span-",
			rc: config.RotationConfig{
				ManualRollover: configoptional.Some(config.ManualRolloverRotation{
					WriteAlias: "my-write-alias",
					ReadAlias:  "my-read-alias",
				}),
			},
			wantWrite: "my-write-alias",
			wantRead:  []string{"my-read-alias"},
		},
		{
			name:   "manual rollover with default aliases",
			prefix: "jaeger-span-",
			rc: config.RotationConfig{
				ManualRollover: configoptional.Some(config.ManualRolloverRotation{}),
			},
			wantWrite: "jaeger-span-write",
			wantRead:  []string{"jaeger-span-read"},
		},
		{
			name:   "auto rollover with explicit aliases",
			prefix: "jaeger-span-",
			rc: config.RotationConfig{
				AutoRollover: configoptional.Some(config.AutoRolloverRotation{
					WriteAlias: "auto-write",
					ReadAlias:  "auto-read",
				}),
			},
			wantWrite: "auto-write",
			wantRead:  []string{"auto-read"},
		},
		{
			name:   "auto rollover with default aliases",
			prefix: "jaeger-span-",
			rc: config.RotationConfig{
				AutoRollover: configoptional.Some(config.AutoRolloverRotation{}),
			},
			wantWrite: "jaeger-span-write",
			wantRead:  []string{"jaeger-span-read"},
		},
		{
			name:   "with remote clusters",
			prefix: "jaeger-span-",
			rc: config.RotationConfig{
				ManualRollover: configoptional.Some(config.ManualRolloverRotation{}),
			},
			remoteClusters: []string{"cluster-a", "cluster-b"},
			wantWrite:      "jaeger-span-write",
			wantRead:       []string{"jaeger-span-read", "cluster-a:jaeger-span-read", "cluster-b:jaeger-span-read"},
		},
		{
			name:      "default (empty config) falls back to daily periodic",
			prefix:    "jaeger-span-",
			rc:        config.RotationConfig{},
			wantWrite: "jaeger-span-2024-03-15",
			wantRead:  []string{"jaeger-span-2024-03-15"},
		},
		{
			name:   "hourly rollover frequency",
			prefix: "jaeger-span-",
			rc: config.RotationConfig{
				Periodic: configoptional.Some(config.PeriodicRotation{
					DateLayout:        "2006-01-02",
					RolloverFrequency: "hour",
				}),
			},
			wantWrite: "jaeger-span-2024-03-15",
			wantRead:  []string{"jaeger-span-2024-03-15"},
		},
		{
			name:   "data stream reads/writes the data stream name",
			prefix: "jaeger-span-",
			rc: config.RotationConfig{
				DataStream: configoptional.Some(config.DataStreamRotation{Name: "jaeger.spans"}),
			},
			wantWrite: "jaeger.spans",
			wantRead:  []string{"jaeger.spans"},
		},
		{
			name:   "data stream with migration read alias",
			prefix: "jaeger-span-",
			rc: config.RotationConfig{
				DataStream: configoptional.Some(config.DataStreamRotation{Name: "jaeger.spans", ReadAlias: "jaeger.spans-read"}),
			},
			wantWrite: "jaeger.spans",
			wantRead:  []string{"jaeger.spans-read"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := BuildRotation(tt.prefix, tt.rc, tt.remoteClusters, logger)
			assert.Equal(t, tt.wantWrite, r.WriteTarget(ts))
			assert.Equal(t, tt.wantRead, r.ReadTargets(ts, ts))
		})
	}
}

func TestResolveSpanDataStreamName(t *testing.T) {
	t.Run("sets the resolved name for a data_stream rotation", func(t *testing.T) {
		rc := config.RotationConfig{DataStream: configoptional.Some(config.DataStreamRotation{})}
		ResolveSpanDataStreamName(&rc, "prod")
		assert.Equal(t, "prod.jaeger.spans", rc.DataStream.Get().Name)
	})
	t.Run("is a no-op for non data_stream rotations", func(t *testing.T) {
		rc := config.RotationConfig{Periodic: configoptional.Some(config.PeriodicRotation{})}
		ResolveSpanDataStreamName(&rc, "prod")
		assert.False(t, rc.DataStream.HasValue())
	})
}
