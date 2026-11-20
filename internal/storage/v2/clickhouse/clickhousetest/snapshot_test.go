// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhousetest

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyQuerySnapshot(t *testing.T) {
	t.Chdir(t.TempDir())
	regenerate := regenerateSnapshots
	t.Cleanup(func() { regenerateSnapshots = regenerate })

	regenerateSnapshots = true
	VerifyQuerySnapshot(t, "SELECT 1", "  SELECT 2  ")
	second, err := os.ReadFile("snapshots/TestVerifyQuerySnapshot_2.sql")
	require.NoError(t, err)
	assert.Equal(t, "SELECT 2\n", string(second))

	regenerateSnapshots = false
	VerifyQuerySnapshot(t, "SELECT 1", "SELECT 2")
}
