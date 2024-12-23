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
		SpanIDUniquifier(),
		SortAttributesAndEvents(),
		SpanHash(),                    // requires SortAttributesAndEvents for deterministic results
		ClockSkew(maxClockSkewAdjust), // adds warnings (which affect SpanHash) on duplicate span IDs
		IPAttribute(),
		ResourceAttributes(),
		SpanLinks(),
	}
}
