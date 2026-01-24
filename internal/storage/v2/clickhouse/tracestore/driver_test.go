// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
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

type testBatch struct {
	driver.Batch
	t          *testing.T
	appended   [][]any
	appendErr  error
	sendCalled bool
	sendErr    error
}

func (tb *testBatch) Append(v ...any) error {
	if tb.appendErr != nil {
		return tb.appendErr
	}
	tb.appended = append(tb.appended, v)
	return nil
}

func (tb *testBatch) Send() error {
	if tb.sendErr != nil {
		return tb.sendErr
	}
	tb.sendCalled = true
	return nil
}

func (*testBatch) Close() error {
	return nil
}

type testQueryResponse struct {
	rows driver.Rows
	err  error
}

type testBatchResponse struct {
	batch *testBatch
	err   error
}

type testDriver struct {
	driver.Conn

	t               *testing.T
	queryResponses  map[string]*testQueryResponse
	batchResponses  map[string]*testBatchResponse
	recordedQueries []string
}

func (t *testDriver) Query(_ context.Context, query string, _ ...any) (driver.Rows, error) {
	t.recordedQueries = append(t.recordedQueries, query)

	for querySubstring, response := range t.queryResponses {
		if strings.Contains(query, querySubstring) {
			return response.rows, response.err
		}
	}

	return nil, nil
}

type testRows[T any] struct {
	driver.Rows

	data     []T
	index    int
	scanErr  error
	scanFn   func(dest any, src T) error
	closeErr error
	rowsErr  error
}

func (tr *testRows[T]) Close() error {
	return tr.closeErr
}

func (tr *testRows[T]) Err() error {
	return tr.rowsErr
}

func (tr *testRows[T]) Next() bool {
	return tr.index < len(tr.data)
}

func (tr *testRows[T]) ScanStruct(dest any) error {
	if tr.scanErr != nil {
		return tr.scanErr
	}
	if tr.index >= len(tr.data) {
		return errors.New("no more rows")
	}
	if tr.scanFn == nil {
		return errors.New("scanFn is not provided")
	}
	err := tr.scanFn(dest, tr.data[tr.index])
	tr.index++
	return err
}

func (tr *testRows[T]) Scan(dest ...any) error {
	if tr.scanErr != nil {
		return tr.scanErr
	}
	if tr.index >= len(tr.data) {
		return errors.New("no more rows")
	}
	if tr.scanFn == nil {
		return errors.New("scanFn is not provided")
	}
	err := tr.scanFn(dest, tr.data[tr.index])
	tr.index++
	return err
}

func (t *testDriver) PrepareBatch(
	_ context.Context,
	query string,
	_ ...driver.PrepareBatchOption,
) (driver.Batch, error) {
	t.recordedQueries = append(t.recordedQueries, query)

	for querySubstring, response := range t.batchResponses {
		if strings.Contains(query, querySubstring) {
			return response.batch, response.err
		}
	}

	return nil, nil
}
