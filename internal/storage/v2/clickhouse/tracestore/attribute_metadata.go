// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

// attributeMetadata maps attribute keys to levels, and each level to a list of types.
// Structure: attributeMetadata[key][level] = []type
// Example: attributeMetadata["http.status"]["span"] = ["int", "str"]
type attributeMetadata map[string]map[string][]string

// getAttributeMetadata retrieves the types stored in ClickHouse for string attributes.
//
// The query service forwards all attribute filters as strings (via AsString()), regardless
// of their actual type. For example:
//   - A bool attribute stored as true becomes the string "true"
//   - An int attribute stored as 123 becomes the string "123"
//
// To query ClickHouse correctly, we need to:
//  1. Look up the actual type(s) from the attribute_metadata table
//  2. Convert the string back to the original type
//  3. Query the appropriate typed column (bool_attributes, int_attributes, etc.)
//
// Since attributes can be stored with different types across different spans
// (e.g. "http.status" could be an int in one span and a string in another),
// the metadata can return multiple types for a single key. We build OR conditions
// to match any of the possible types.
//
// Only string-typed attributes from the query are looked up, since other types
// (bool, int, double, etc.) are already correctly typed in the query parameters.
func (r *Reader) getAttributeMetadata(attributes pcommon.Map) (attributeMetadata, error) {
	query, args := buildSelectAttributeMetadataQuery(attributes)
	rows, err := r.conn.Query(context.Background(), query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query attribute metadata: %w", err)
	}
	defer rows.Close()

	metadata := make(attributeMetadata)
	for rows.Next() {
		var attrMeta dbmodel.AttributeMetadata
		if err := rows.ScanStruct(&attrMeta); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		if metadata[attrMeta.AttributeKey] == nil {
			metadata[attrMeta.AttributeKey] = make(map[string][]string)
		}
		metadata[attrMeta.AttributeKey][attrMeta.Level] = append(metadata[attrMeta.AttributeKey][attrMeta.Level], attrMeta.Type)
	}
	return metadata, nil
}

func buildSelectAttributeMetadataQuery(attributes pcommon.Map) (string, []any) {
	var q strings.Builder
	q.WriteString(sql.SelectAttributeMetadata)
	args := []any{}

	var placeholders []string
	for key, attr := range attributes.All() {
		if attr.Type() == pcommon.ValueTypeStr {
			placeholders = append(placeholders, "?")
			args = append(args, key)
		}
	}

	if len(placeholders) > 0 {
		q.WriteString(" WHERE attribute_key IN (")
		q.WriteString(strings.Join(placeholders, ", "))
		q.WriteString(")")
	}
	q.WriteString(" GROUP BY attribute_key, type, level")
	return q.String(), args
}
