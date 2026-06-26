// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import "time"

const (
	SpanTemplateName       = "jaeger-span"
	ServiceTemplateName    = "jaeger-service"
	DependencyTemplateName = "jaeger-dependencies"
	SamplingTemplateName   = "jaeger-sampling"

	SpanIndexBaseName       = SpanTemplateName + "-"
	ServiceIndexBaseName    = ServiceTemplateName + "-"
	DependencyIndexBaseName = DependencyTemplateName + "-"
	SamplingIndexBaseName   = SamplingTemplateName + "-"

	// SpanDataStreamBaseName is the dot-notation name of the spans data stream.
	// Dot-notation (vs. "jaeger-span-") aligns with ES/OpenSearch conventions and
	// enables the "@custom" component-template override pattern. See RFC 0004 §3.1.
	SpanDataStreamBaseName = "jaeger.spans"
)

// WriteOpType represents the Elasticsearch bulk operation type.
type WriteOpType string

const (
	// WriteOpIndex is the standard "index" operation (upsert semantics).
	WriteOpIndex WriteOpType = "index"

	// WriteOpCreate is the "create" operation (fail if document exists).
	// Used by data streams.
	WriteOpCreate WriteOpType = "create"
)

// Rotation defines how indices are named for reading and writing.
// Each index type (spans, services) gets its own Rotation instance.
type Rotation interface {
	// WriteTarget returns the index name to write to for the given span time.
	WriteTarget(spanTime time.Time) string

	// ReadTargets returns the list of index names to search for the given time range.
	ReadTargets(startTime, endTime time.Time) []string

	// WriteOpType returns the Elasticsearch bulk operation type for write operations.
	WriteOpType() WriteOpType
}
