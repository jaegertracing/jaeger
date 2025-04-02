// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"time"

	adjuster2 "github.com/jaegertracing/jaeger/cmd/query/app/querysvc/internal/adjuster"
)

// StandardAdjusters is a list of model adjusters applied by the query service
// before returning the data to the API clients.
func StandardAdjusters(maxClockSkewAdjust time.Duration) []adjuster2.Adjuster {
	return []adjuster2.Adjuster{
		adjuster2.ZipkinSpanIDUniquifier(),
		adjuster2.SortTagsAndLogFields(),
		adjuster2.DedupeBySpanHash(),            // requires SortTagsAndLogFields for deterministic results
		adjuster2.ClockSkew(maxClockSkewAdjust), // adds warnings (which affect SpanHash) on dupe span IDs
		adjuster2.IPTagAdjuster(),
		adjuster2.OTelTagAdjuster(),
		adjuster2.SpanReferences(),
		adjuster2.ParentReference(),
	}
}
