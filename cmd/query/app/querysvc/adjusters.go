// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"time"

	"github.com/jaegertracing/jaeger/internal/adjuster"
)

// StandardAdjusters is a list of model adjusters applied by the query service
// before returning the data to the API clients.
func StandardAdjusters(maxClockSkewAdjust time.Duration) []adjuster.Adjuster {
	return []adjuster.Adjuster{
		adjuster.ZipkinSpanIDUniquifier(),
		adjuster.SortTagsAndLogFields(),
		adjuster.DedupeBySpanHash(),            // requires SortTagsAndLogFields for deterministic results
		adjuster.ClockSkew(maxClockSkewAdjust), // adds warnings (which affect SpanHash) on dupe span IDs
		adjuster.IPTagAdjuster(),
		adjuster.OTelTagAdjuster(),
		adjuster.SpanReferences(),
		adjuster.ParentReference(),
	}
}
