// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
//	REGENERATE_SNAPSHOTS=true go test -v ./internal/storage/v2/clickhouse/tracestore/...
var regenerateSnapshots = os.Getenv("REGENERATE_SNAPSHOTS") == "true"

func verifyQuerySnapshot(t *testing.T, query string) {
	testName := t.Name()
	snapshotFile := filepath.Join(snapshotLocation, testName+".sql")
	query = strings.TrimSpace(query)
	if regenerateSnapshots {
		dir := filepath.Dir(snapshotFile)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create snapshot directory: %v", err)
		}
		os.WriteFile(snapshotFile, []byte(query), 0o644)
	}
	snapshot, err := os.ReadFile(snapshotFile)
	require.NoError(t, err)
	assert.Equal(t, string(snapshot), query, "comparing against stored snapshot. Use REGENERATE_SNAPSHOTS=true to rebuild snapshots.")
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

type testDriver struct {
	driver.Conn

	t             *testing.T
	rows          driver.Rows
	err           error
	batch         *testBatch
	recordedQuery string
}

func (t *testDriver) Query(_ context.Context, query string, _ ...any) (driver.Rows, error) {
	t.recordedQuery = query
	return t.rows, t.err
}

type testRows[T any] struct {
	driver.Rows

	data     []T
	index    int
	scanErr  error
	scanFn   func(dest any, src T) error
	closeErr error
}

func (tr *testRows[T]) Close() error {
	return tr.closeErr
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
	t.recordedQuery = query
	if t.err != nil {
		return nil, t.err
	}
	return t.batch, nil
}
