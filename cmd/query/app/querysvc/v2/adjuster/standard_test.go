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
	assert.IsType(t, DeduplicateClientServerSpanIDs(), adjusters[0])
	assert.IsType(t, SortCollections(), adjusters[1])
	assert.IsType(t, DeduplicateSpans(), adjusters[2])
	assert.IsType(t, CorrectClockSkew(maxClockSkewAdjust), adjusters[3])
	assert.IsType(t, NormalizeIPAttributes(), adjusters[4])
	assert.IsType(t, MoveLibraryAttributes(), adjusters[5])
	assert.IsType(t, RemoveEmptySpanLinks(), adjusters[6])
}
