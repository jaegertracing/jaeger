// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
)

// TestAttributeLayout_SelectSpans_Map snapshots the statement that reads spans from a table in
// the map layout. The statement of the nested layout is in the snapshots of the reader tests.
func TestAttributeLayout_SelectSpans_Map(t *testing.T) {
	reader := NewReader(&clickhousetest.Driver{}, MapLayout{}, ReaderConfig{})

	query, _ := reader.buildGetTracesQuery(tracestore.GetTraceParams{})

	clickhousetest.VerifyQuerySnapshot(t, query)
}

// TestAttributeLayout_ColumnTypeSQL snapshots the declaration of one attribute group in each
// layout.
func TestAttributeLayout_ColumnTypeSQL(t *testing.T) {
	clickhousetest.VerifyQuerySnapshot(t,
		"bool_attributes "+NestedLayout{}.ColumnTypeSQL(AttributeTypeBool),
		"bool_attributes "+MapLayout{}.ColumnTypeSQL(AttributeTypeBool),
	)
}

func TestAttributeLayoutByName(t *testing.T) {
	for _, want := range attributeLayouts {
		layout, err := AttributeLayoutByName(want.Name())
		require.NoError(t, err)
		assert.Equal(t, want, layout)
	}

	_, err := AttributeLayoutByName("json")
	require.EqualError(t, err, `attribute_layout must be nested or map, not "json"`)
}

// TestAttributeLayout_Declares pins which column types of system.columns each layout takes
// for a group of its own.
func TestAttributeLayout_Declares(t *testing.T) {
	tests := []struct {
		columnType string
		layout     AttributeLayout
	}{
		{
			columnType: "Array(String)",
			layout:     NestedLayout{},
		},
		{
			columnType: "Nested(key String, value String)",
			layout:     NestedLayout{},
		},
		{
			columnType: "Map(String, String)",
			layout:     MapLayout{},
		},
	}
	for _, tt := range tests {
		for _, layout := range attributeLayouts {
			assert.Equal(t, layout == tt.layout, layout.Declares(tt.columnType), "%s as %s", tt.columnType, layout.Name())
		}
	}
}

// TestAttributeLayout_KeysSQL checks the keys of a group that is a column of the table and
// of a group that is stored once per event.
func TestAttributeLayout_KeysSQL(t *testing.T) {
	resourceStr := attributeGroup(expression.LevelResource, pcommon.ValueTypeStr)
	eventStr := attributeGroup(expression.LevelEvent, pcommon.ValueTypeStr)

	assert.Equal(t, "resource_str_attributes.key", NestedLayout{}.KeysSQL(resourceStr))
	assert.Equal(t, "events.str_attributes.key", NestedLayout{}.KeysSQL(eventStr))
	assert.Equal(t, "mapKeys(resource_str_attributes)", MapLayout{}.KeysSQL(resourceStr))
	assert.Equal(t, "arrayMap(m -> mapKeys(m), events.str_attributes)", MapLayout{}.KeysSQL(eventStr))
}
