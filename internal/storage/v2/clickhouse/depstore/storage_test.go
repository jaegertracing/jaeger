// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package depstore

import (
	"context"
	"errors"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
)

type testDriver struct {
	driver.Conn

	rows driver.Rows
	err  error
}

func (t *testDriver) Query(_ context.Context, _ string, _ ...any) (driver.Rows, error) {
	return t.rows, t.err
}

type testRows struct {
	driver.Rows

	data    []Dependency
	index   int
	scanErr error
}

func (*testRows) Close() error {
	return nil
}

func (f *testRows) Next() bool {
	return f.index < len(f.data)
}

func (f *testRows) ScanStruct(dest any) error {
	if f.scanErr != nil {
		return f.scanErr
	}
	if f.index >= len(f.data) {
		return errors.New("no more rows")
	}
	d, ok := dest.(*Dependency)
	if !ok {
		return errors.New("wrong type in ScanStruct")
	}
	*d = f.data[f.index]
	f.index++
	return nil
}

func TestGetDependencies(t *testing.T) {
	tests := []struct {
		name        string
		conn        *testDriver
		expected    []model.DependencyLink
		expectError string
	}{
		{
			name: "successfully returns dependencies",
			conn: &testDriver{
				rows: &testRows{
					data: []Dependency{
						{Parent: "serviceA", Child: "serviceB", CallCount: 10, Source: "sourceA"},
						{Parent: "serviceB", Child: "serviceC", CallCount: 5, Source: "sourceB"},
					},
				},
			},
			expected: []model.DependencyLink{
				{Parent: "serviceA", Child: "serviceB", CallCount: 10, Source: "sourceA"},
				{Parent: "serviceB", Child: "serviceC", CallCount: 5, Source: "sourceB"},
			},
		},
		{
			name:        "query error",
			conn:        &testDriver{err: assert.AnError},
			expectError: "failed to query dependencies",
		},
		{
			name: "scan error",
			conn: &testDriver{
				rows: &testRows{
					data:    []Dependency{{Parent: "serviceA", Child: "serviceB", CallCount: 10, Source: "sourceA"}},
					scanErr: assert.AnError,
				},
			},
			expectError: "failed to scan row",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			storage := NewStorage(test.conn)

			result, err := storage.GetDependencies(context.Background(), depstore.QueryParameters{})

			if test.expectError != "" {
				require.ErrorContains(t, err, test.expectError)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expected, result)
			}
		})
	}
}
