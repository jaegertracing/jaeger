// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/v1/api/metricstore"
)

func TestCalculateTimeRange(t *testing.T) {
	endTime := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	lookback := 1 * time.Hour
	startTime := endTime.Add(-lookback)

	tests := []struct {
		name                        string
		params                      *metricstore.BaseQueryParameters
		wantErr                     string
		wantStartTimeMillis         int64
		wantEndTimeMillis           int64
		wantExtendedStartTimeMillis int64
	}{
		{
			name:    "nil params",
			params:  nil,
			wantErr: "invalid parameters",
		},
		{
			name:    "nil EndTime",
			params:  &metricstore.BaseQueryParameters{},
			wantErr: "invalid parameters",
		},
		{
			name: "nil Lookback",
			params: &metricstore.BaseQueryParameters{
				EndTime: &endTime,
			},
			wantErr: "invalid parameters",
		},
		{
			name: "nil RatePer",
			params: &metricstore.BaseQueryParameters{
				EndTime:  &endTime,
				Lookback: &lookback,
			},
			wantErr: "invalid parameters",
		},
		{
			name: "RatePer=5m extends 5m before startTime",
			params: func() *metricstore.BaseQueryParameters {
				ratePer := 5 * time.Minute
				return &metricstore.BaseQueryParameters{
					EndTime:  &endTime,
					Lookback: &lookback,
					RatePer:  &ratePer,
				}
			}(),
			wantStartTimeMillis:         startTime.UnixMilli(),
			wantEndTimeMillis:           endTime.UnixMilli(),
			wantExtendedStartTimeMillis: startTime.Add(-5 * time.Minute).UnixMilli(),
		},
		{
			name: "RatePer=10m extends 10m before startTime (same as old default)",
			params: func() *metricstore.BaseQueryParameters {
				ratePer := 10 * time.Minute
				return &metricstore.BaseQueryParameters{
					EndTime:  &endTime,
					Lookback: &lookback,
					RatePer:  &ratePer,
				}
			}(),
			wantStartTimeMillis:         startTime.UnixMilli(),
			wantEndTimeMillis:           endTime.UnixMilli(),
			wantExtendedStartTimeMillis: startTime.Add(-10 * time.Minute).UnixMilli(),
		},
		{
			name: "RatePer=30m extends 30m before startTime",
			params: func() *metricstore.BaseQueryParameters {
				ratePer := 30 * time.Minute
				return &metricstore.BaseQueryParameters{
					EndTime:  &endTime,
					Lookback: &lookback,
					RatePer:  &ratePer,
				}
			}(),
			wantStartTimeMillis:         startTime.UnixMilli(),
			wantEndTimeMillis:           endTime.UnixMilli(),
			wantExtendedStartTimeMillis: startTime.Add(-30 * time.Minute).UnixMilli(),
		},
		{
			name: "RatePer=1h extends 1h before startTime",
			params: func() *metricstore.BaseQueryParameters {
				ratePer := 1 * time.Hour
				return &metricstore.BaseQueryParameters{
					EndTime:  &endTime,
					Lookback: &lookback,
					RatePer:  &ratePer,
				}
			}(),
			wantStartTimeMillis:         startTime.UnixMilli(),
			wantEndTimeMillis:           endTime.UnixMilli(),
			wantExtendedStartTimeMillis: startTime.Add(-1 * time.Hour).UnixMilli(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr, err := calculateTimeRange(tt.params)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantStartTimeMillis, tr.startTimeMillis, "startTimeMillis")
			assert.Equal(t, tt.wantEndTimeMillis, tr.endTimeMillis, "endTimeMillis")
			assert.Equal(t, tt.wantExtendedStartTimeMillis, tr.extendedStartTimeMillis,
				"extendedStartTimeMillis should equal startTime - RatePer")
		})
	}
}
