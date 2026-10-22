// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONSafeFloat(t *testing.T) {
	assert.Equal(t, "NaN", JSONSafeFloat(math.NaN()))
	assert.Equal(t, "Infinity", JSONSafeFloat(math.Inf(1)))
	assert.Equal(t, "-Infinity", JSONSafeFloat(math.Inf(-1)))

	finite, ok := JSONSafeFloat(1.5).(float64)
	require.True(t, ok)
	assert.InDelta(t, 1.5, finite, 0)
	zero, ok := JSONSafeFloat(0).(float64)
	require.True(t, ok)
	assert.InDelta(t, 0.0, zero, 0)
}

func TestNonFiniteFloatToken(t *testing.T) {
	nan, ok := NonFiniteFloatToken("NaN")
	require.True(t, ok)
	assert.True(t, math.IsNaN(nan))

	pinf, ok := NonFiniteFloatToken("Infinity")
	require.True(t, ok)
	assert.True(t, math.IsInf(pinf, 1))

	ninf, ok := NonFiniteFloatToken("-Infinity")
	require.True(t, ok)
	assert.True(t, math.IsInf(ninf, -1))

	for _, s := range []string{"", "Inf", "-Inf", "+Infinity", "nan", "1.5", "NaN "} {
		_, ok := NonFiniteFloatToken(s)
		assert.False(t, ok, s)
	}
}
