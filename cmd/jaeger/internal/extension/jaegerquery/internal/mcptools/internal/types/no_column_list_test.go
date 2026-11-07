// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package types

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoColumnListInSchemas holds the schema of each tool that refuses a column list to the
// NoColumnList sentence, which a struct tag can only repeat.
func TestNoColumnListInSchemas(t *testing.T) {
	for _, input := range []any{FindSpansInput{}, FindTracesInput{}} {
		field, ok := reflect.TypeOf(input).FieldByName("SQL")
		require.True(t, ok)
		assert.Contains(t, field.Tag.Get("jsonschema"), NoColumnList, "%T", input)
	}
}
