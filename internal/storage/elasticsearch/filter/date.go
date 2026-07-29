// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"time"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
)

// ByDate filter indices by creationTime, return indices that were created before certain date.
func ByDate(indices []esclient.Index, beforeThisDate time.Time) []esclient.Index {
	var filtered []esclient.Index
	for _, in := range indices {
		if in.CreationTime.Before(beforeThisDate) {
			filtered = append(filtered, in)
		}
	}
	return filtered
}
