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
	maxTraceSize := "512Mi"
	adjusters := StandardAdjusters(maxTraceSize, maxClockSkewAdjust)

	assert.Len(t, adjusters, 8, "Expected 8 adjusters")
	assert.IsType(t, DeduplicateClientServerSpanIDs(), adjusters[0])
	assert.IsType(t, SortCollections(), adjusters[1])
	assert.IsType(t, DeduplicateSpans(), adjusters[2])
	assert.IsType(t, CorrectMaxSize(maxTraceSize), adjusters[3])
	assert.IsType(t, CorrectClockSkew(maxClockSkewAdjust), adjusters[4])
	assert.IsType(t, NormalizeIPAttributes(), adjusters[5])
	assert.IsType(t, MoveLibraryAttributes(), adjusters[6])
	assert.IsType(t, RemoveEmptySpanLinks(), adjusters[7])
}
