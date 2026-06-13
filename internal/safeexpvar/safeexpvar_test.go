// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package safeexpvar

import (
	"expvar"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestSetInt(t *testing.T) {
	// Test with a new variable
	name := "metrics-test-1"
	value := int64(42)

	SetInt(name, value)

	// Retrieve the variable and check its value
	v := expvar.Get(name)
	assert.NotNil(t, v, "expected variable %s to be created", name)
	expInt, ok := v.(*expvar.Int)
	require.True(t, ok, "expected variable %s to be of type *expvar.Int", name)
	assert.Equal(t, value, expInt.Value())
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
