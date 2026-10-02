// Copyright (c) 2026 The Jaeger Authors.
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
	endTime := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	lookback := time.Hour
	startTime := endTime.Add(-lookback)

	tr, err := calculateTimeRange(&metricstore.BaseQueryParameters{
		EndTime:  &endTime,
		Lookback: &lookback,
	})
	require.NoError(t, err)
	assert.Equal(t, startTime.UnixMilli(), tr.startTimeMillis)
	assert.Equal(t, endTime.UnixMilli(), tr.endTimeMillis)
	assert.Equal(t, startTime.UnixMilli(), tr.extendedStartTimeMillis)
}

func TestCalculateRateTimeRange(t *testing.T) {
	endTime := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	lookback := time.Hour
	startTime := endTime.Add(-lookback)

	for _, ratePer := range []time.Duration{5 * time.Minute, 10 * time.Minute, 30 * time.Minute, time.Hour} {
		t.Run(ratePer.String(), func(t *testing.T) {
			tr, err := calculateRateTimeRange(&metricstore.BaseQueryParameters{
				EndTime:  &endTime,
				Lookback: &lookback,
				RatePer:  &ratePer,
			})
			require.NoError(t, err)
			assert.Equal(t, startTime.UnixMilli(), tr.startTimeMillis)
			assert.Equal(t, endTime.UnixMilli(), tr.endTimeMillis)
			assert.Equal(t, startTime.Add(-ratePer).UnixMilli(), tr.extendedStartTimeMillis,
				"extendedStartTimeMillis should equal startTime - RatePer")
		})
	}
}
