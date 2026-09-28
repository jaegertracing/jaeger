// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIdsQuerySource(t *testing.T) {
	src, err := NewIdsQuery("a1", "b2").Source()
	require.NoError(t, err)
	b, err := json.Marshal(src)
	require.NoError(t, err)
	assert.JSONEq(t, `{"ids":{"values":["a1","b2"]}}`, string(b))
}
