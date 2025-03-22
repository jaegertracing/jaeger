// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package filter

import (
	"time"

	"github.com/jaegertracing/jaeger/internal/storage/es/client"
)

// ByDate filter indices by creationTime, return indices that were created before certain date.
func ByDate(indices []client.Index, beforeThisDate time.Time) []client.Index {
	var filtered []client.Index
	for _, in := range indices {
		if in.CreationTime.Before(beforeThisDate) {
			filtered = append(filtered, in)
		}
	}
	return filtered
}
