// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}

func readJSONBytes(t *testing.T, filename string) []byte {
	t.Helper()
	path := filepath.Join(".", filename)
	bytes, err := os.ReadFile(path)
	require.NoError(t, err, "Failed to read file %s", filename)
	return bytes
}
