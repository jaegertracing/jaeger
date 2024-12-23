package adjuster

import (
	"time"
)

// StandardAdjusters is a list of adjusters applied by the query service
// before returning the data to the API clients.
func StandardAdjusters(maxClockSkewAdjust time.Duration) []Adjuster {
	return []Adjuster{
		SpanIDUniquifier(),
		SortAttributesAndEvents(),
		SpanHash(),                    // requires SortTagsAndLogFields for deterministic results
		ClockSkew(maxClockSkewAdjust), // adds warnings (which affect SpanHash) on dupe span IDs
		IPAttribute(),
		ResourceAttributes(),
		SpanLinks(),
	}
}
