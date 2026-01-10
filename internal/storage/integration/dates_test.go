// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func Test_dateOffsetNormalizer(t *testing.T) {
	origTime := time.Date(
		2024, time.January, 10,
		14, 30, 45, 123456789,
		time.UTC,
	)
	origTs := pcommon.NewTimestampFromTime(origTime)
	dayOffset := -2
	normalizer := newDateOffsetNormalizer(dayOffset)
	normalizedTs := normalizer.normalize(origTs)
	normalizedTime := normalizedTs.AsTime()
	expectedDate := time.Now().UTC().AddDate(0, 0, dayOffset)
	expectedTime := time.Date(
		expectedDate.Year(),
		expectedDate.Month(),
		expectedDate.Day(),
		origTime.Hour(),
		origTime.Minute(),
		origTime.Second(),
		origTime.Nanosecond(),
		time.UTC,
	)
	assert.Equal(t, expectedTime, normalizedTime)
}
