// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestPeriodicRotation_WriteTarget(t *testing.T) {
	r := NewPeriodicRotation("jaeger-span-", "2006-01-02", -24*time.Hour)
	date := time.Date(1995, time.April, 21, 22, 8, 41, 0, time.UTC)
	assert.Equal(t, "jaeger-span-1995-04-21", r.WriteTarget(date))
}

func TestPeriodicRotation_WriteTarget_Hourly(t *testing.T) {
	r := NewPeriodicRotation("jaeger-span-", "2006-01-02-15", -1*time.Hour)
	date := time.Date(2019, time.October, 10, 5, 0, 0, 0, time.UTC)
	assert.Equal(t, "jaeger-span-2019-10-10-05", r.WriteTarget(date))
}

func TestPeriodicRotation_ReadTargets(t *testing.T) {
	r := NewPeriodicRotation("jaeger-span-", "2006-01-02", -24*time.Hour)
	today := time.Date(1995, time.April, 21, 4, 12, 19, 95, time.UTC)
	yesterday := today.AddDate(0, 0, -1)

	tests := []struct {
		name     string
		start    time.Time
		end      time.Time
		expected []string
	}{
		{
			name:     "same day",
			start:    today.Add(-time.Millisecond),
			end:      today,
			expected: []string{"jaeger-span-1995-04-21"},
		},
		{
			name:  "spans two days",
			start: today.Add(-13 * time.Hour),
			end:   today,
			expected: []string{
				"jaeger-span-1995-04-21",
				"jaeger-span-1995-04-20",
			},
		},
		{
			name:  "spans three days",
			start: yesterday.Add(-24 * time.Hour),
			end:   today,
			expected: []string{
				"jaeger-span-1995-04-21",
				"jaeger-span-1995-04-20",
				"jaeger-span-1995-04-19",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, r.ReadTargets(tt.start, tt.end))
		})
	}
}

func TestPeriodicRotation_ZeroRolloverFrequencyDefaultsToDaily(t *testing.T) {
	r := NewPeriodicRotation("jaeger-span-", "2006-01-02", 0)
	today := time.Date(1995, time.April, 21, 4, 12, 19, 95, time.UTC)
	result := r.ReadTargets(today.Add(-25*time.Hour), today)
	assert.Equal(t, []string{"jaeger-span-1995-04-21", "jaeger-span-1995-04-20"}, result)
}

func TestPeriodicRotation_OpType(t *testing.T) {
	r := NewPeriodicRotation("jaeger-span-", "2006-01-02", -24*time.Hour)
	assert.Equal(t, "index", r.OpType())
}

func TestPeriodicRotation_UseTimeRangeFilter(t *testing.T) {
	r := NewPeriodicRotation("jaeger-span-", "2006-01-02", -24*time.Hour)
	assert.False(t, r.UseTimeRangeFilter())
}

func TestAliasRotation_WriteTarget(t *testing.T) {
	r := NewAliasRotation("jaeger-span-write", "jaeger-span-read")
	date := time.Date(2024, time.June, 18, 10, 0, 0, 0, time.UTC)
	assert.Equal(t, "jaeger-span-write", r.WriteTarget(date))
}

func TestAliasRotation_ReadTargets(t *testing.T) {
	r := NewAliasRotation("jaeger-span-write", "jaeger-span-read")
	start := time.Date(2024, time.June, 17, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, time.June, 18, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, []string{"jaeger-span-read"}, r.ReadTargets(start, end))
}

func TestAliasRotation_OpType(t *testing.T) {
	r := NewAliasRotation("jaeger-span-write", "jaeger-span-read")
	assert.Equal(t, "index", r.OpType())
}

func TestAliasRotation_UseTimeRangeFilter(t *testing.T) {
	r := NewAliasRotation("jaeger-span-write", "jaeger-span-read")
	assert.True(t, r.UseTimeRangeFilter())
}

func TestRemoteClusterRotation(t *testing.T) {
	inner := NewPeriodicRotation("jaeger-span-", "2006-01-02", -24*time.Hour)
	r := NewRemoteClusterRotation(inner, []string{"cluster_one", "cluster_two"})

	date := time.Date(1995, time.April, 21, 4, 0, 0, 0, time.UTC)

	t.Run("WriteTarget delegates to inner", func(t *testing.T) {
		assert.Equal(t, "jaeger-span-1995-04-21", r.WriteTarget(date))
	})

	t.Run("ReadTargets adds remote clusters", func(t *testing.T) {
		expected := []string{
			"jaeger-span-1995-04-21",
			"cluster_one:jaeger-span-1995-04-21",
			"cluster_two:jaeger-span-1995-04-21",
		}
		assert.Equal(t, expected, r.ReadTargets(date, date))
	})

	t.Run("OpType delegates to inner", func(t *testing.T) {
		assert.Equal(t, "index", r.OpType())
	})

	t.Run("UseTimeRangeFilter delegates to inner", func(t *testing.T) {
		assert.False(t, r.UseTimeRangeFilter())
	})
}

func TestRemoteClusterRotation_WithAlias(t *testing.T) {
	inner := NewAliasRotation("jaeger-span-write", "jaeger-span-read")
	r := NewRemoteClusterRotation(inner, []string{"cluster_one"})

	expected := []string{
		"jaeger-span-read",
		"cluster_one:jaeger-span-read",
	}
	assert.Equal(t, expected, r.ReadTargets(time.Now(), time.Now()))
	assert.True(t, r.UseTimeRangeFilter())
}

func TestRemoteClusterRotation_NoClusters(t *testing.T) {
	inner := NewPeriodicRotation("jaeger-span-", "2006-01-02", -24*time.Hour)
	r := NewRemoteClusterRotation(inner, nil)

	date := time.Date(1995, time.April, 21, 4, 0, 0, 0, time.UTC)
	assert.Equal(t, []string{"jaeger-span-1995-04-21"}, r.ReadTargets(date, date))
}

func TestLoggingRotation_NoDebug(t *testing.T) {
	inner := NewPeriodicRotation("jaeger-span-", "2006-01-02", -24*time.Hour)
	r := NewLoggingRotation(inner, zap.NewNop())
	// Should return the inner unwrapped since debug is not enabled
	assert.Equal(t, inner, r)
}

func TestLoggingRotation_WithDebug(t *testing.T) {
	logger := zaptest.NewLogger(t, zaptest.Level(zap.DebugLevel))
	inner := NewPeriodicRotation("jaeger-span-", "2006-01-02", -24*time.Hour)
	r := NewLoggingRotation(inner, logger)
	// Should wrap the inner with logging
	assert.IsType(t, &LoggingRotation{}, r)

	date := time.Date(1995, time.April, 21, 4, 0, 0, 0, time.UTC)
	assert.Equal(t, "jaeger-span-1995-04-21", r.WriteTarget(date))
	assert.Equal(t, []string{"jaeger-span-1995-04-21"}, r.ReadTargets(date, date))
	assert.Equal(t, "index", r.OpType())
	assert.False(t, r.UseTimeRangeFilter())
}

func TestBuildRotation(t *testing.T) {
	date := time.Date(2019, time.October, 10, 5, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		opts           RotationBuilderOptions
		expectedWrite  string
		expectedRead   []string
		expectedFilter bool
	}{
		{
			name: "periodic daily",
			opts: RotationBuilderOptions{
				IndexPrefix:       "jaeger-span-",
				DateLayout:        "2006-01-02",
				RolloverFrequency: -24 * time.Hour,
			},
			expectedWrite:  "jaeger-span-2019-10-10",
			expectedRead:   []string{"jaeger-span-2019-10-10"},
			expectedFilter: false,
		},
		{
			name: "periodic hourly",
			opts: RotationBuilderOptions{
				IndexPrefix:       "jaeger-span-",
				DateLayout:        "2006-01-02-15",
				RolloverFrequency: -1 * time.Hour,
			},
			expectedWrite:  "jaeger-span-2019-10-10-05",
			expectedRead:   []string{"jaeger-span-2019-10-10-05"},
			expectedFilter: false,
		},
		{
			name: "alias with default suffixes",
			opts: RotationBuilderOptions{
				IndexPrefix:         "jaeger-span-",
				UseReadWriteAliases: true,
				WriteSuffix:         "write",
				ReadSuffix:          "read",
			},
			expectedWrite:  "jaeger-span-write",
			expectedRead:   []string{"jaeger-span-read"},
			expectedFilter: true,
		},
		{
			name: "alias with custom suffixes",
			opts: RotationBuilderOptions{
				IndexPrefix:         "jaeger-span-",
				UseReadWriteAliases: true,
				WriteSuffix:         "archive",
				ReadSuffix:          "archive",
			},
			expectedWrite:  "jaeger-span-archive",
			expectedRead:   []string{"jaeger-span-archive"},
			expectedFilter: true,
		},
		{
			name: "explicit aliases override everything",
			opts: RotationBuilderOptions{
				IndexPrefix:         "jaeger-span-",
				UseReadWriteAliases: true,
				WriteSuffix:         "write",
				ReadSuffix:          "read",
				ExplicitWriteAlias:  "custom-span-write",
				ExplicitReadAlias:   "custom-span-read",
			},
			expectedWrite:  "custom-span-write",
			expectedRead:   []string{"custom-span-read"},
			expectedFilter: true,
		},
		{
			name: "periodic with remote clusters",
			opts: RotationBuilderOptions{
				IndexPrefix:        "jaeger-span-",
				DateLayout:         "2006-01-02",
				RolloverFrequency:  -24 * time.Hour,
				RemoteReadClusters: []string{"cluster_one", "cluster_two"},
			},
			expectedWrite: "jaeger-span-2019-10-10",
			expectedRead: []string{
				"jaeger-span-2019-10-10",
				"cluster_one:jaeger-span-2019-10-10",
				"cluster_two:jaeger-span-2019-10-10",
			},
			expectedFilter: false,
		},
		{
			name: "alias with remote clusters",
			opts: RotationBuilderOptions{
				IndexPrefix:         "jaeger-span-",
				UseReadWriteAliases: true,
				WriteSuffix:         "write",
				ReadSuffix:          "read",
				RemoteReadClusters:  []string{"cluster_one", "cluster_two"},
			},
			expectedWrite: "jaeger-span-write",
			expectedRead: []string{
				"jaeger-span-read",
				"cluster_one:jaeger-span-read",
				"cluster_two:jaeger-span-read",
			},
			expectedFilter: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := BuildRotation(tt.opts)
			assert.Equal(t, tt.expectedWrite, r.WriteTarget(date))
			assert.Equal(t, tt.expectedRead, r.ReadTargets(date, date))
			assert.Equal(t, "index", r.OpType())
			assert.Equal(t, tt.expectedFilter, r.UseTimeRangeFilter())
		})
	}
}
