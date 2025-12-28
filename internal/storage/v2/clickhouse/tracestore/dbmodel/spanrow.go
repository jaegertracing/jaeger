// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// SpanRow represents a single record in the ClickHouse `spans` table.
//
// Complex attributes are non-primitive OTLP types that require special serialization
// before being stored. These types are encoded as follows:
//
//   - pcommon.ValueTypeBytes:
//     Represents raw byte data. The value is Base64-encoded and stored as a string.
//     Keys for this type are prefixed with `@bytes@`.
//
//   - pcommon.ValueTypeSlice:
//     Represents an OTLP slice (array). The value is serialized to JSON and stored
//     as a string. Keys for this type are prefixed with `@slice@`.
//
//   - pcommon.ValueTypeMap:
//     Represents an OTLP map. The value is serialized to JSON and stored
//     as a string. Keys for this type are prefixed with `@map@`.
type SpanRow struct {
	// --- Span ---
	ID              string
	TraceID         string
	TraceState      string
	ParentSpanID    string
	Name            string
	Kind            string
	StartTime       time.Time
	StatusCode      string
	StatusMessage   string
	Duration        int64
	Attributes      Attributes
	EventNames      []string
	EventTimestamps []time.Time
	EventAttributes Attributes2D
	LinkTraceIDs    []string
	LinkSpanIDs     []string
	LinkTraceStates []string
	LinkAttributes  Attributes2D

	// --- Resource ---
	ServiceName        string
	ResourceAttributes Attributes

	// --- Scope ---
	ScopeName       string
	ScopeVersion    string
	ScopeAttributes Attributes
}

type Attributes struct {
	Keys   []string
	Values []string
	Types  []string
}

type Attributes2D struct {
	Keys   [][]string
	Values [][]string
	Types  [][]string
}

func ScanRow(rows driver.Rows) (*SpanRow, error) {
	var sr SpanRow
	err := rows.Scan(
		&sr.ID,
		&sr.TraceID,
		&sr.TraceState,
		&sr.ParentSpanID,
		&sr.Name,
		&sr.Kind,
		&sr.StartTime,
		&sr.StatusCode,
		&sr.StatusMessage,
		&sr.Duration,
		&sr.Attributes.Keys,
		&sr.Attributes.Values,
		&sr.Attributes.Types,
		&sr.EventNames,
		&sr.EventTimestamps,
		&sr.EventAttributes.Keys,
		&sr.EventAttributes.Values,
		&sr.EventAttributes.Types,
		&sr.LinkTraceIDs,
		&sr.LinkSpanIDs,
		&sr.LinkTraceStates,
		&sr.LinkAttributes.Keys,
		&sr.LinkAttributes.Values,
		&sr.LinkAttributes.Types,
		&sr.ServiceName,
		&sr.ResourceAttributes.Keys,
		&sr.ResourceAttributes.Values,
		&sr.ResourceAttributes.Types,
		&sr.ScopeName,
		&sr.ScopeVersion,
		&sr.ScopeAttributes.Keys,
		&sr.ScopeAttributes.Values,
		&sr.ScopeAttributes.Types,
	)
	if err != nil {
		return nil, err
	}
	return &sr, nil
}
