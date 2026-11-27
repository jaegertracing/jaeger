// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhousetest

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

// regenerateSnapshots makes VerifyQuerySnapshot rewrite the snapshot files from the queries it
// is given:
//
//	REGENERATE_SNAPSHOTS=true go test ./internal/storage/v2/clickhouse/...
var regenerateSnapshots = os.Getenv("REGENERATE_SNAPSHOTS") == "true"

// VerifyQuerySnapshot compares each query with its snapshot file in the snapshots directory of
// the calling test's package. The files are named after the test and the position of the
// query, starting from 1: snapshots/<TestName>_1.sql, snapshots/<TestName>_2.sql.
func VerifyQuerySnapshot(t *testing.T, queries ...string) {
	t.Helper()
	for i, query := range queries {
		snapshotFile := filepath.Join(snapshotLocation, t.Name()+"_"+strconv.Itoa(i+1)+".sql")
		query = strings.TrimSpace(query)
		if regenerateSnapshots {
			require.NoError(t, os.MkdirAll(filepath.Dir(snapshotFile), 0o750))
			require.NoError(t, os.WriteFile(snapshotFile, []byte(query+"\n"), 0o600))
		}
		snapshot, err := os.ReadFile(snapshotFile)
		require.NoError(t, err)
		assert.Equal(t, strings.TrimSpace(string(snapshot)), query, "comparing against stored snapshot. Use REGENERATE_SNAPSHOTS=true to rebuild snapshots.")
	}
}
