// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

// attrTypes holds the value types for an attribute key at different levels.
// These types are populated by the materialized view defined in
// internal/storage/v2/clickhouse/sql/create_attribute_metadata_mv.sql
type attrTypes struct {
	resource []pcommon.ValueType
	scope    []pcommon.ValueType
	span     []pcommon.ValueType
}

// attributeMetadata maps attribute keys to their types per level.
// Example: attributeMetadata["http.status"].span = ["int", "str"]
type attributeMetadata map[string]attrTypes

// getAttributeMetadata retrieves the types stored in ClickHouse for attributes that arrive as strings.
//
// Query Flow:
// 1. HTTP/gRPC API receives tag filters as query parameters (e.g., ?tag=http.status:200)
// 2. The query parser parses them into map[string]string
// 3. The map gets converted to a pcommon.Map using PutStr() for all values
// 4. This function receives those string-typed attributes and looks up their actual storage types
//
// The query APIs (both HTTP and gRPC) only accept string values for tag filters, regardless
// of how attributes were originally stored in ClickHouse. For example:
//   - A bool attribute stored as true arrives as the string "true"
//   - An int attribute stored as 123 arrives as the string "123"
//   - A string attribute stored as "ok" arrives as the string "ok"
//
// To query ClickHouse correctly, we need to:
//  1. Look up the actual type(s) from the attribute_metadata table
//  2. Convert the string back to the original type for filtering
//  3. Query the appropriate typed column (bool_attributes, int_attributes, etc.)
//
// Since attributes can be stored with different types across different spans
// (e.g. "http.status" could be an int in one span and a string in another),
// the metadata can return multiple types for a single key. We build OR conditions
// to match any of the possible types.
//
// Only string-typed attributes from pcommon.Map are looked up since those are the ones
// that originated from the query API's string-only input format.
func (r *Reader) getAttributeMetadata(ctx context.Context, attributes pcommon.Map) (attributeMetadata, error) {
	q := newQueryBuilder()
	q.appendSelectAttributeMetadataQuery(attributes)
	query, args := q.build()

	metadata := make(attributeMetadata)
	if len(args) == 0 {
		// No string attributes to look up
		return metadata, nil
	}

	rows, err := r.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query attribute metadata: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var attrMeta dbmodel.AttributeMetadata
		if err := rows.ScanStruct(&attrMeta); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		levels := metadata[attrMeta.AttributeKey]
		switch attrMeta.Level {
		case "resource":
			levels.resource = append(levels.resource, jptrace.StringToValueType(attrMeta.Type))
		case "scope":
			levels.scope = append(levels.scope, jptrace.StringToValueType(attrMeta.Type))
		case "span":
			levels.span = append(levels.span, jptrace.StringToValueType(attrMeta.Type))
		default:
			return nil, fmt.Errorf("unknown attribute level %q", attrMeta.Level)
		}
		metadata[attrMeta.AttributeKey] = levels
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating attribute metadata rows: %w", err)
	}
	return metadata, nil
}
