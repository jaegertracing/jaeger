// Copyright (c) 2026 The Jaeger Authors.
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

func TestTermsAggregationExecutionHint(t *testing.T) {
	src, err := NewTermsAggregation("traceID").
		Size(10).
		ExecutionHint(ExecutionHintMap).
		Source()
	require.NoError(t, err)
	b, err := json.Marshal(src)
	require.NoError(t, err)
	assert.JSONEq(t, `{"terms":{"field":"traceID","size":10,"execution_hint":"map"}}`, string(b))
}

func TestTermsAggregationExecutionHintGlobalOrdinals(t *testing.T) {
	src, err := NewTermsAggregation("traceID").
		ExecutionHint(ExecutionHintGlobalOrdinals).
		Source()
	require.NoError(t, err)
	b, err := json.Marshal(src)
	require.NoError(t, err)
	assert.JSONEq(t, `{"terms":{"field":"traceID","execution_hint":"global_ordinals"}}`, string(b))
}

func TestTermsAggregationOmitsUnsetExecutionHint(t *testing.T) {
	src, err := NewTermsAggregation("traceID").Source()
	require.NoError(t, err)
	b, err := json.Marshal(src)
	require.NoError(t, err)
	assert.JSONEq(t, `{"terms":{"field":"traceID"}}`, string(b))
}
