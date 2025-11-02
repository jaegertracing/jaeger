// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestToRow(t *testing.T) {
	now := time.Now().UTC()
	duration := 2 * time.Second

	rs := createTestResource()
	sc := createTestScope()
	span := createTestSpan(now, duration)

	expected := createTestSpanRow(t, now, duration)

	row := ToRow(rs, sc, span)
	require.Equal(t, expected, row)
}
