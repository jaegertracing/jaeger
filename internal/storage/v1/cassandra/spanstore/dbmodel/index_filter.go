// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

const (
	// DurationIndex represents the flag for indexing by duration.
	DurationIndex = iota

	// ServiceIndex represents the flag for indexing by service.
	ServiceIndex

	// OperationIndex represents the flag for indexing by service-operation.
	OperationIndex
)

// IndexFilter filters out any spans that should not be indexed depending on the index specified.
type IndexFilter func(span *Span, index int) bool

// DefaultIndexFilter is a filter that indexes everything.
var DefaultIndexFilter = func(_ *Span, _ /* index */ int) bool {
	return true
}
