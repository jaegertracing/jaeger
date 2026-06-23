// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package indices

import (
	"strings"
	"time"
)

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

// DataStreamName builds the fully-qualified data stream name for the given raw
// index prefix, joining with a dot so the prefix participates in the dot-notation
// hierarchy (e.g. "" -> "jaeger.spans", "prod" -> "prod.jaeger.spans"). See
// RFC 0004 §3.1.
//
// A trailing separator is normalized away first, so that a prefix written with the
// legacy "-" separator or an explicit "." (both accepted by IndexPrefix.Apply)
// produces the same name as the bare prefix: "prod", "prod-" and "prod." all yield
// "prod.jaeger.spans". Internal dashes are preserved ("my-team" -> "my-team.jaeger.spans").
func DataStreamName(indexPrefix, base string) string {
	prefix := strings.TrimRight(indexPrefix, ".-")
	if prefix == "" {
		return base
	}
	return prefix + "." + base
}

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
