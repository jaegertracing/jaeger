// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestTraceID_ToOTEL(t *testing.T) {
	id, err := TraceID("1").ToOTEL()
	require.NoError(t, err)
	assert.Equal(t, pcommon.TraceID([16]byte{15: 1}), id, "a short id is left-padded")

	id, err = TraceID("0102030405060708090a0b0c0d0e0f10").ToOTEL()
	require.NoError(t, err)
	assert.Equal(t, pcommon.TraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}), id)

	_, err = TraceID("0102030405060708090a0b0c0d0e0f1011").ToOTEL()
	require.ErrorContains(t, err, "too long")

	_, err = TraceID("zz").ToOTEL()
	require.Error(t, err, "non-hex is rejected")
}

func TestSpanID_ToOTEL(t *testing.T) {
	id, err := SpanID("a").ToOTEL()
	require.NoError(t, err)
	assert.Equal(t, pcommon.SpanID([8]byte{7: 0xa}), id, "a short id is left-padded")

	_, err = SpanID("0102030405060708ff").ToOTEL()
	require.ErrorContains(t, err, "too long")

	_, err = SpanID("zz").ToOTEL()
	require.Error(t, err, "non-hex is rejected")
}
