// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestPcommonTraceIDFromBytes(t *testing.T) {
	full := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	got, err := pcommonTraceIDFromBytes(full)
	require.NoError(t, err)
	assert.Equal(t, pcommon.TraceID(full), got)

	short := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	got, err = pcommonTraceIDFromBytes(short)
	require.NoError(t, err)
	assert.Equal(t, pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8}), got)

	_, err = pcommonTraceIDFromBytes([]byte{1})
	require.Error(t, err)
	_, err = pcommonTraceIDFromBytes(append(full, 17))
	require.Error(t, err)
}
