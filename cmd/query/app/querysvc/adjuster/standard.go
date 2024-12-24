// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"time"
)

// StandardAdjusters returns a list of adjusters applied by the query service
// before returning the data to the API clients.
func StandardAdjusters(maxClockSkewAdjust time.Duration) []Adjuster {
	return []Adjuster{
		DeduplicateClientServerSpanIDs(),
		SortCollections(),
		// DeduplicateSpans depends on SortCollections running first
		DeduplicateSpans(),
		CorrectClockSkew(maxClockSkewAdjust),
		NormalizeIPAttributes(),
		MoveLibraryAttributes(),
		RemoveEmptySpanLinks(),
	}
}
