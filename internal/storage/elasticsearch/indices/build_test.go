// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			name: "periodic rotation with default date layout",
			rc: config.RotationConfig{
				Periodic: configoptional.Some(config.PeriodicRotation{
					RolloverFrequency: "day",
				}),
			},
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
			name: "monthly rollover frequency",
			rc: config.RotationConfig{
				Periodic: configoptional.Some(config.PeriodicRotation{
					DateLayout:        "2006-01",
					RolloverFrequency: "month",
				}),
			},
			wantWrite: "jaeger-span-2024-03",
			wantRead:  []string{"jaeger-span-2024-03"},
		},
		{
			name: "yearly rollover frequency",
			rc: config.RotationConfig{
				Periodic: configoptional.Some(config.PeriodicRotation{
					DateLayout:        "2006",
					RolloverFrequency: "year",
				}),
			},
			wantWrite: "jaeger-span-2024",
			wantRead:  []string{"jaeger-span-2024"},
		},
		{
			name: "monthly rollover frequency with omitted date layout defaults to monthly layout",
			rc: config.RotationConfig{
				Periodic: configoptional.Some(config.PeriodicRotation{
					RolloverFrequency: "month",
				}),
			},
			wantWrite: "jaeger-span-2024-03",
			wantRead:  []string{"jaeger-span-2024-03"},
		},
		{
			name: "yearly rollover frequency with omitted date layout defaults to yearly layout",
			rc: config.RotationConfig{
				Periodic: configoptional.Some(config.PeriodicRotation{
					RolloverFrequency: "year",
				}),
			},
			wantWrite: "jaeger-span-2024",
			wantRead:  []string{"jaeger-span-2024"},
		},
		{
			name: "hourly rollover frequency with omitted date layout defaults to hourly layout",
			rc: config.RotationConfig{
				Periodic: configoptional.Some(config.PeriodicRotation{
					RolloverFrequency: "hour",
				}),
			},
			wantWrite: "jaeger-span-2024-03-15-00",
			wantRead:  []string{"jaeger-span-2024-03-15-00"},
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

func TestBuildRotation_PeriodicRolloverFrequencyDuration(t *testing.T) {
	logger := zap.NewNop()

	frequencies := []string{"hour", "day", "month", "year", "unrecognized-defaults-to-day"}
	for _, frequency := range frequencies {
		t.Run(frequency, func(t *testing.T) {
			rc := config.RotationConfig{
				Periodic: configoptional.Some(config.PeriodicRotation{
					DateLayout:        "2006-01-02",
					RolloverFrequency: frequency,
				}),
			}
			r := BuildRotation("", config.SpanIndexName, rc, nil, logger)

			pr, ok := r.(*PeriodicRotation)
			require.True(t, ok, "expected BuildRotation to return a *PeriodicRotation when Periodic is set")
			assert.Equal(t, -config.RolloverFrequencyDuration(frequency), pr.rolloverFrequency)
		})
	}
}

func TestBuildRotation_MonthlyFrequencyWithoutDateLayoutDoesNotSkipIndices(t *testing.T) {
	logger := zap.NewNop()
	rc := config.RotationConfig{
		Periodic: configoptional.Some(config.PeriodicRotation{
			RolloverFrequency: "month",
		}),
	}
	r := BuildRotation("", config.SpanIndexName, rc, nil, logger)

	assert.Equal(t, "jaeger-span-2024-03", r.WriteTarget(time.Date(2024, time.March, 15, 0, 0, 0, 0, time.UTC)))

	start := time.Date(2024, time.January, 10, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, time.March, 20, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, []string{
		"jaeger-span-2024-03",
		"jaeger-span-2024-02",
		"jaeger-span-2024-01",
	}, r.ReadTargets(start, end))
}

func TestIndexToDataStreamName(t *testing.T) {
	tests := []struct {
		indexName string
		want      string
	}{
		{config.SpanIndexName, "jaeger.spans"},
		{config.ServiceIndexName, "jaeger.services"},
		{config.DependencyIndexName, "jaeger.dependencies"},
		{config.SamplingIndexName, "jaeger.sampling"},
		{"custom-index-name", "custom.index.name"},
	}
	for _, tt := range tests {
		t.Run(tt.indexName, func(t *testing.T) {
			assert.Equal(t, tt.want, indexToDataStreamName(tt.indexName))
		})
	}
}
