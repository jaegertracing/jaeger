// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTermsAggregationSource(t *testing.T) {
	src, err := NewTermsAggregation("operationName").Size(10).Source()
	require.NoError(t, err)
	b, err := json.Marshal(src)
	require.NoError(t, err)
	assert.JSONEq(t, `{"terms":{"field":"operationName","size":10}}`, string(b))
}

func TestTermsAggregationOmitsUnsetSize(t *testing.T) {
	src, err := NewTermsAggregation("operationName").Source()
	require.NoError(t, err)
	b, err := json.Marshal(src)
	require.NoError(t, err)
	assert.JSONEq(t, `{"terms":{"field":"operationName"}}`, string(b))
}
