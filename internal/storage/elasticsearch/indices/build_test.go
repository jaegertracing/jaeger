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
		indexPrefix    config.IndexPrefix
		rc             config.RotationConfig
		remoteClusters []string
		wantWrite      string
		wantRead       []string
	}{
		{
			name: "periodic rotation",
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
			name: "manual rollover with explicit aliases",
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
			name: "manual rollover with default aliases",
			rc: config.RotationConfig{
				ManualRollover: configoptional.Some(config.ManualRolloverRotation{}),
			},
			wantWrite: "jaeger-span-write",
			wantRead:  []string{"jaeger-span-read"},
		},
		{
			name: "auto rollover with explicit aliases",
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
			name: "auto rollover with default aliases",
			rc: config.RotationConfig{
				AutoRollover: configoptional.Some(config.AutoRolloverRotation{}),
			},
			wantWrite: "jaeger-span-write",
			wantRead:  []string{"jaeger-span-read"},
		},
		{
			name: "with remote clusters",
			rc: config.RotationConfig{
				ManualRollover: configoptional.Some(config.ManualRolloverRotation{}),
			},
			remoteClusters: []string{"cluster-a", "cluster-b"},
			wantWrite:      "jaeger-span-write",
			wantRead:       []string{"jaeger-span-read", "cluster-a:jaeger-span-read", "cluster-b:jaeger-span-read"},
		},
		{
			name:      "default (empty config) falls back to daily periodic",
			rc:        config.RotationConfig{},
			wantWrite: "jaeger-span-2024-03-15",
			wantRead:  []string{"jaeger-span-2024-03-15"},
		},
		{
			name: "hourly rollover frequency",
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
			name: "data stream derives its name from the index prefix",
			rc: config.RotationConfig{
				DataStream: configoptional.Some(config.DataStreamRotation{}),
			},
			wantWrite: "jaeger.spans",
			wantRead:  []string{"jaeger.spans"},
		},
		{
			name:        "data stream with index prefix and migration read alias",
			indexPrefix: "prod",
			rc: config.RotationConfig{
				DataStream: configoptional.Some(config.DataStreamRotation{ReadAlias: "jaeger-legacy-read-alias"}),
			},
			wantWrite: "prod.jaeger.spans",
			wantRead:  []string{"jaeger-legacy-read-alias"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := BuildRotation(tt.indexPrefix, config.SpanIndexName, tt.rc, tt.remoteClusters, logger)
			assert.Equal(t, tt.wantWrite, r.WriteTarget(ts))
			assert.Equal(t, tt.wantRead, r.ReadTargets(ts, ts))
		})
	}
}
