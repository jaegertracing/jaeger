// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNestedQuerySource(t *testing.T) {
	inner := NewBoolQuery().Must(
		NewMatchQuery("tags.key", "http.status_code"),
		NewRegexpQuery("tags.value", "200"),
	)
	src, err := NewNestedQuery("tags", inner).Source()
	require.NoError(t, err)
	nested := src.(map[string]any)["nested"].(map[string]any)
	assert.Equal(t, "tags", nested["path"])
	assert.Contains(t, nested, "query")
}

func TestNestedQueryIgnoreUnmapped(t *testing.T) {
	tests := []struct {
		name           string
		ignoreUnmapped bool
	}{
		{name: "unset leaves the option out", ignoreUnmapped: false},
		{name: "set renders the option", ignoreUnmapped: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			src, err := NewNestedQuery("tags", NewMatchQuery("tags.key", "k")).
				IgnoreUnmapped(test.ignoreUnmapped).
				Source()
			require.NoError(t, err)
			nested := src.(map[string]any)["nested"].(map[string]any)
			if test.ignoreUnmapped {
				assert.Equal(t, true, nested["ignore_unmapped"])
			} else {
				assert.NotContains(t, nested, "ignore_unmapped")
			}
		})
	}
}

func TestNestedQueryPropagatesInnerError(t *testing.T) {
	_, err := NewNestedQuery("tags", errQuery{}).Source()
	require.ErrorIs(t, err, errBadQuery)
}
