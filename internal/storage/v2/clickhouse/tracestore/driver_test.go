// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const snapshotLocation = "./snapshots/"

// REGENERATE_SNAPSHOTS=true go test -v ./internal/storage/v2/clickhouse/tracestore/...
var regenerateSnapshots = os.Getenv("REGENERATE_SNAPSHOTS") == "true"

// verifyQuerySnapshot verifies SQL queries against snapshot files.
func verifyQuerySnapshot(t *testing.T, queries ...string) {
	t.Helper()

	testName := t.Name()

	for i, query := range queries {
		index := i + 1

		snapshotFile := filepath.Join(
			snapshotLocation,
			testName+"_"+strconv.Itoa(index)+".sql",
		)

		query = strings.TrimSpace(query)

		if regenerateSnapshots {
			if err := os.MkdirAll(filepath.Dir(snapshotFile), 0o755); err != nil {
				t.Fatalf("failed to create snapshot directory: %v", err)
			}

			if err := os.WriteFile(snapshotFile, []byte(query+"\n"), 0o644); err != nil {
				t.Fatalf("failed to write snapshot file: %v", err)
			}
		}

		snapshot, err := os.ReadFile(snapshotFile)
		require.NoError(t, err)

		assert.Equal(
			t,
			strings.TrimSpace(string(snapshot)),
			query,
			"snapshot mismatch. Use REGENERATE_SNAPSHOTS=true to regenerate snapshots.",
		)
	}
}