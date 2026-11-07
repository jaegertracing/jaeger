// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoColumnListInSchemas checks that the schema tag of each tool that refuses a column
// list contains the NoColumnList sentence. A struct tag is a string literal and cannot refer to
// the constant, so the tag spells the sentence out and this test keeps the two copies equal.
func TestNoColumnListInSchemas(t *testing.T) {
	for _, input := range []any{FindSpansInput{}, FindTracesInput{}} {
		field, ok := reflect.TypeOf(input).FieldByName("SQL")
		require.True(t, ok)
		assert.Contains(t, field.Tag.Get("jsonschema"), NoColumnList, "%T", input)
	}
}
