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

const (
	snapshotLocation = "./snapshots/"
)

// Snapshots can be regenerated via:
//
// REGENERATE_SNAPSHOTS=true go test -v ./internal/storage/v2/clickhouse/tracestore/...
var regenerateSnapshots = os.Getenv("REGENERATE_SNAPSHOTS") == "true"

// verifyQuerySnapshot verifies one or more SQL queries against their snapshot files.
// Queries are indexed sequentially starting from 1, and snapshot files are named as:
//
//	snapshots/<TestName>_1.sql, snapshots/<TestName>_2.sql, etc.
//
// The order of queries passed to this function determines their index and filename.
// For example, verifyQuerySnapshot(t, query1, query2, query3) will verify against:
//
//	snapshots/<TestName>_1.sql, snapshots/<TestName>_2.sql, snapshots/<TestName>_3.sql
func verifyQuerySnapshot(t *testing.T, queries ...string) {
	testName := t.Name()
	for i, query := range queries {
		index := i + 1
		snapshotFile := filepath.Join(snapshotLocation, testName+"_"+strconv.Itoa(index)+".sql")
		query = strings.TrimSpace(query)
		if regenerateSnapshots {
			dir := filepath.Dir(snapshotFile)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("failed to create snapshot directory: %v", err)
			}
			if err := os.WriteFile(snapshotFile, []byte(query+"\n"), 0o644); err != nil {
				t.Fatalf("failed to write snapshot file: %v", err)
			}
		}
		snapshot, err := os.ReadFile(snapshotFile)
		require.NoError(t, err)
		assert.Equal(t, strings.TrimSpace(string(snapshot)), query, "comparing against stored snapshot. Use REGENERATE_SNAPSHOTS=true to rebuild snapshots.")
	}
}
