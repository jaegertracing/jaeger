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

func TestNestedQueryPropagatesInnerError(t *testing.T) {
	_, err := NewNestedQuery("tags", errQuery{}).Source()
	require.ErrorIs(t, err, errBadQuery)
}
