// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import "time"

// Rotation defines how indices are named for reading and writing.
// Each index type (spans, services) gets its own Rotation instance.
type Rotation interface {
	// WriteTarget returns the index name to write to for the given span time.
	WriteTarget(spanTime time.Time) string

	// ReadTargets returns the list of index names to search for the given time range.
	ReadTargets(startTime, endTime time.Time) []string

	// WriteOpType returns the Elasticsearch operation type for write operations.
	WriteOpType() string
}
