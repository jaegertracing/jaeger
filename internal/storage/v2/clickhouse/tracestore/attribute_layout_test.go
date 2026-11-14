// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
)

// TestAttributeLayout_SelectSpans_Map snapshots the statement that reads spans from a table in
// the map layout. The statement of the nested layout is in the snapshots of the reader tests.
func TestAttributeLayout_SelectSpans_Map(t *testing.T) {
	reader := NewReader(&clickhousetest.Driver{}, ReaderConfig{AttributeLayout: MapLayout{}})

	query, _ := reader.buildGetTracesQuery(tracestore.GetTraceParams{})

	clickhousetest.VerifyQuerySnapshot(t, query)
}

// TestAttributeLayout_GroupType snapshots the declaration of one attribute group in each
// layout.
func TestAttributeLayout_GroupType(t *testing.T) {
	clickhousetest.VerifyQuerySnapshot(t,
		"bool_attributes "+NestedLayout{}.GroupType("Bool"),
		"bool_attributes "+MapLayout{}.GroupType("Bool"),
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
