// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import (
	"time"

	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
)

// Rotation defines how indices are named for reading and writing.
// Each index type (spans, services) gets its own Rotation instance.
type Rotation interface {
	// WriteTarget returns the index name to write to for the given span time.
	WriteTarget(spanTime time.Time) string

	// ReadTargets returns the list of index names to search for the given time range.
	// For a periodic range wider than the request-line budget, the list is a wildcard
	// pattern rather than one concrete name per period.
	ReadTargets(startTime, endTime time.Time) []string

	// ExactTargets returns concrete index names covering the time range, newest first.
	// It never substitutes a wildcard. A periodic range wider than the request-line
	// budget still yields concrete names, capped so a huge lookback cannot allocate one
	// name per period; the names past the cap are the older ones. Search callers use
	// ReadTargets instead.
	ExactTargets(startTime, endTime time.Time) []string

	// WriteOpType returns the Elasticsearch bulk operation type for write operations.
	WriteOpType() es.WriteOpType

	// RequiresDocumentTimestamp reports whether documents written to this target
	// must carry an @timestamp field (required by data streams).
	RequiresDocumentTimestamp() bool
}
