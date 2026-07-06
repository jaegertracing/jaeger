// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package query

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTermQuerySource(t *testing.T) {
	src, err := NewTermQuery("serviceName", "test-service").Source()
	require.NoError(t, err)
	b, err := json.Marshal(src)
	require.NoError(t, err)
	assert.JSONEq(t, `{"term":{"serviceName":"test-service"}}`, string(b))
}
