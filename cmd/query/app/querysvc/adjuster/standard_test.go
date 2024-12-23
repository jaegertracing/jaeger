// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStandardAdjusters(t *testing.T) {
	maxClockSkewAdjust := 10 * time.Second
	adjusters := StandardAdjusters(maxClockSkewAdjust)

	assert.Len(t, adjusters, 7, "Expected 7 adjusters")
	assert.IsType(t, SpanIDUniquifier(), adjusters[0])
	assert.IsType(t, SortAttributesAndEvents(), adjusters[1])
	assert.IsType(t, SpanHash(), adjusters[2])
	assert.IsType(t, ClockSkew(maxClockSkewAdjust), adjusters[3])
	assert.IsType(t, IPAttribute(), adjusters[4])
	assert.IsType(t, ResourceAttributes(), adjusters[5])
	assert.IsType(t, SpanLinks(), adjusters[6])
}
