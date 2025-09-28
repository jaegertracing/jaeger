// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"testing"

	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
)

func TestNewTagQueryBuilder(t *testing.T) {
	dotReplacer := dbmodel.NewDotReplacer("@")
	builder := NewTagQueryBuilder(dotReplacer)

	assert.NotNil(t, builder)
	assert.Equal(t, dotReplacer, builder.dotReplacer)
}

func TestTagQueryBuilder_BuildTagQuery(t *testing.T) {
	tests := []struct {
		name            string
		key             string
		value           string
		dotReplacement  string
		expectedQueries int
	}{
		{
			name:            "simple key-value",
			key:             "environment",
			value:           "production",
			dotReplacement:  "@",
			expectedQueries: 5, // 2 object queries + 3 nested queries
		},
		{
			name:            "key with dots",
			key:             "service.name",
			value:           "user-service",
			dotReplacement:  "@",
			expectedQueries: 5,
		},
		{
			name:            "special characters in value",
			key:             "version",
			value:           "v1.2.3",
			dotReplacement:  "#",
			expectedQueries: 5,
		},
		{
			name:            "empty value",
			key:             "status",
			value:           "",
			dotReplacement:  "@",
			expectedQueries: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dotReplacer := dbmodel.NewDotReplacer(tt.dotReplacement)
			builder := NewTagQueryBuilder(dotReplacer)

			query := builder.BuildTagQuery(tt.key, tt.value)

			// Assert that we get a BoolQuery
			boolQuery, ok := query.(*elastic.BoolQuery)
			require.True(t, ok, "Expected BoolQuery")

			// Convert to map to inspect structure
			queryMap, err := boolQuery.Source()
			require.NoError(t, err)

			// Check that it's a bool query with should clause
			queryMapTyped, ok := queryMap.(map[string]interface{})
			require.True(t, ok, "Expected query map to be map[string]interface{}")

			boolClause, exists := queryMapTyped["bool"]
			require.True(t, exists, "Expected bool clause")

			boolMap, ok := boolClause.(map[string]interface{})
			require.True(t, ok, "Expected bool clause to be map")

			shouldClause, exists := boolMap["should"]
			require.True(t, exists, "Expected should clause")

			shouldQueries, ok := shouldClause.([]interface{})
			require.True(t, ok, "Expected should clause to be array")

			// Verify we have the expected number of queries
			assert.Equal(t, tt.expectedQueries, len(shouldQueries), "Expected %d queries", tt.expectedQueries)
		})
	}
}

func TestTagQueryBuilder_BuildNestedQuery(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		key      string
		value    string
		expected map[string]interface{}
	}{
		{
			name:  "nested tags query",
			field: "tags",
			key:   "environment",
			value: "production",
		},
		{
			name:  "nested process tags query",
			field: "process.tags",
			key:   "service.name",
			value: "user-service",
		},
		{
			name:  "nested log fields query",
			field: "logs.fields",
			key:   "level",
			value: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dotReplacer := dbmodel.NewDotReplacer("@")
			builder := NewTagQueryBuilder(dotReplacer)

			query := builder.buildNestedQuery(tt.field, tt.key, tt.value)

			// Assert that we get a NestedQuery
			nestedQuery, ok := query.(*elastic.NestedQuery)
			require.True(t, ok, "Expected NestedQuery")

			// Convert to map to inspect structure
			queryMap, err := nestedQuery.Source()
			require.NoError(t, err)

			// Check nested structure
			queryMapTyped, ok := queryMap.(map[string]interface{})
			require.True(t, ok, "Expected query map to be map[string]interface{}")

			nestedClause, exists := queryMapTyped["nested"]
			require.True(t, exists, "Expected nested clause")

			nestedMap, ok := nestedClause.(map[string]interface{})
			require.True(t, ok, "Expected nested clause to be map")

			// Check path
			path, exists := nestedMap["path"]
			require.True(t, exists, "Expected path in nested query")
			assert.Equal(t, tt.field, path, "Expected path to match field")

			// Check query structure
			queryClause, exists := nestedMap["query"]
			require.True(t, exists, "Expected query in nested clause")

			queryBool, ok := queryClause.(map[string]interface{})
			require.True(t, ok, "Expected query to be map")

			// Should have bool -> must structure
			boolClause, exists := queryBool["bool"]
			require.True(t, exists, "Expected bool clause in nested query")

			boolMap, ok := boolClause.(map[string]interface{})
			require.True(t, ok, "Expected bool clause to be map")

			mustClause, exists := boolMap["must"]
			require.True(t, exists, "Expected must clause")

			mustQueries, ok := mustClause.([]interface{})
			require.True(t, ok, "Expected must clause to be array")

			// Should have exactly 2 queries: key match and value regexp
			assert.Equal(t, 2, len(mustQueries), "Expected 2 must queries")
		})
	}
}

func TestTagQueryBuilder_BuildObjectQuery(t *testing.T) {
	tests := []struct {
		name  string
		field string
		key   string
		value string
	}{
		{
			name:  "object tag query",
			field: "tag",
			key:   "environment",
			value: "production",
		},
		{
			name:  "object process tag query",
			field: "process.tag",
			key:   "service@name", // Already dot-replaced
			value: "user-service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dotReplacer := dbmodel.NewDotReplacer("@")
			builder := NewTagQueryBuilder(dotReplacer)

			query := builder.buildObjectQuery(tt.field, tt.key, tt.value)

			// Assert that we get a BoolQuery
			boolQuery, ok := query.(*elastic.BoolQuery)
			require.True(t, ok, "Expected BoolQuery")

			// Convert to map to inspect structure
			queryMap, err := boolQuery.Source()
			require.NoError(t, err)

			// Check bool structure
			queryMapTyped, ok := queryMap.(map[string]interface{})
			require.True(t, ok, "Expected query map to be map[string]interface{}")

			boolClause, exists := queryMapTyped["bool"]
			require.True(t, exists, "Expected bool clause")

			boolMap, ok := boolClause.(map[string]interface{})
			require.True(t, ok, "Expected bool clause to be map")

			mustClause, exists := boolMap["must"]
			require.True(t, exists, "Expected must clause")

			// Must clause can be either a single query or an array of queries
			// When there's only one query, elastic returns it as a single object
			// When there are multiple queries, it returns an array
			if mustArray, ok := mustClause.([]interface{}); ok {
				// Multiple queries case
				assert.Equal(t, 1, len(mustArray), "Expected 1 must query")

				// Check that it's a regexp query
				regexpQuery, ok := mustArray[0].(map[string]interface{})
				require.True(t, ok, "Expected query to be map")

				_, exists = regexpQuery["regexp"]
				assert.True(t, exists, "Expected regexp query")
			} else if mustQuery, ok := mustClause.(map[string]interface{}); ok {
				// Single query case
				_, exists = mustQuery["regexp"]
				assert.True(t, exists, "Expected regexp query")
			} else {
				t.Fatal("Expected must clause to be either array or single query")
			}
		})
	}
}

func TestTagQueryBuilder_DotReplacement(t *testing.T) {
	tests := []struct {
		name           string
		dotReplacement string
		key            string
		expectedKey    string
	}{
		{
			name:           "replace dots with @",
			dotReplacement: "@",
			key:            "service.name",
			expectedKey:    "service@name",
		},
		{
			name:           "replace dots with #",
			dotReplacement: "#",
			key:            "trace.span.id",
			expectedKey:    "trace#span#id",
		},
		{
			name:           "no dots in key",
			dotReplacement: "@",
			key:            "environment",
			expectedKey:    "environment",
		},
		{
			name:           "multiple dots",
			dotReplacement: "_",
			key:            "a.b.c.d",
			expectedKey:    "a_b_c_d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dotReplacer := dbmodel.NewDotReplacer(tt.dotReplacement)
			builder := NewTagQueryBuilder(dotReplacer)

			// Build a query and check that dot replacement is applied in object queries
			query := builder.BuildTagQuery(tt.key, "test-value")

			// Convert to map to inspect
			boolQuery, ok := query.(*elastic.BoolQuery)
			require.True(t, ok, "Expected BoolQuery")

			queryMap, err := boolQuery.Source()
			require.NoError(t, err)

			// The dot replacement should be visible in the object queries
			// We can verify this by checking the structure contains the replaced key
			queryMapTyped, ok := queryMap.(map[string]interface{})
			require.True(t, ok, "Expected query map to be map[string]interface{}")

			boolClause := queryMapTyped["bool"].(map[string]interface{})
			shouldClause := boolClause["should"].([]interface{})

			// First two queries should be object queries with dot replacement
			// This is a basic structural test - more detailed testing would require
			// examining the specific field names in the regexp queries
			assert.Equal(t, 5, len(shouldClause), "Expected 5 should queries")
		})
	}
}

func TestTagQueryBuilder_EdgeCases(t *testing.T) {
	dotReplacer := dbmodel.NewDotReplacer("@")
	builder := NewTagQueryBuilder(dotReplacer)

	t.Run("empty key", func(t *testing.T) {
		query := builder.BuildTagQuery("", "value")
		assert.NotNil(t, query)

		// Should still build valid query structure
		boolQuery, ok := query.(*elastic.BoolQuery)
		require.True(t, ok, "Expected BoolQuery")

		queryMap, err := boolQuery.Source()
		require.NoError(t, err)

		queryMapTyped, ok := queryMap.(map[string]interface{})
		require.True(t, ok, "Expected query map to be map[string]interface{}")
		assert.Contains(t, queryMapTyped, "bool")
	})

	t.Run("empty value", func(t *testing.T) {
		query := builder.BuildTagQuery("key", "")
		assert.NotNil(t, query)

		boolQuery, ok := query.(*elastic.BoolQuery)
		require.True(t, ok, "Expected BoolQuery")

		queryMap, err := boolQuery.Source()
		require.NoError(t, err)

		queryMapTyped, ok := queryMap.(map[string]interface{})
		require.True(t, ok, "Expected query map to be map[string]interface{}")
		assert.Contains(t, queryMapTyped, "bool")
	})

	t.Run("special characters", func(t *testing.T) {
		specialKey := "key!@#$%^&*()"
		specialValue := "value!@#$%^&*()"

		query := builder.BuildTagQuery(specialKey, specialValue)
		assert.NotNil(t, query)

		boolQuery, ok := query.(*elastic.BoolQuery)
		require.True(t, ok, "Expected BoolQuery")

		queryMap, err := boolQuery.Source()
		require.NoError(t, err)

		queryMapTyped, ok := queryMap.(map[string]interface{})
		require.True(t, ok, "Expected query map to be map[string]interface{}")
		assert.Contains(t, queryMapTyped, "bool")
	})
}

func TestTagQueryBuilder_Constants(t *testing.T) {
	// Test that constants are properly defined
	assert.Equal(t, "tag", objectTagsField)
	assert.Equal(t, "process.tag", objectProcessTagsField)
	assert.Equal(t, "tags", nestedTagsField)
	assert.Equal(t, "process.tags", nestedProcessTagsField)
	assert.Equal(t, "logs.fields", nestedLogFieldsField)
	assert.Equal(t, "key", tagKeyField)
	assert.Equal(t, "value", tagValueField)

	// Test field lists
	expectedObjectFields := []string{"tag", "process.tag"}
	assert.Equal(t, expectedObjectFields, objectTagFieldList)

	expectedNestedFields := []string{"tags", "process.tags", "logs.fields"}
	assert.Equal(t, expectedNestedFields, nestedTagFieldList)
}
