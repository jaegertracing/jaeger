// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

type testDriver struct {
	driver.Conn

	rows driver.Rows
	err  error
}

func (t *testDriver) Query(_ context.Context, _ string, _ ...any) (driver.Rows, error) {
	return t.rows, t.err
}

type testRows[T any] struct {
	driver.Rows

	data    []T
	index   int
	scanErr error
	scanFn  func(dest any, src T) error
}

func (*testRows[T]) Close() error {
	return nil
}

func (f *testRows[T]) Next() bool {
	return f.index < len(f.data)
}

func (f *testRows[T]) ScanStruct(dest any) error {
	if f.scanErr != nil {
		return f.scanErr
	}
	if f.index >= len(f.data) {
		return errors.New("no more rows")
	}
	if f.scanFn == nil {
		return errors.New("scanFn is not provided")
	}
	err := f.scanFn(dest, f.data[f.index])
	f.index++
	return err
}

func TestGetServices(t *testing.T) {
	tests := []struct {
		name        string
		conn        *testDriver
		expected    []string
		expectError string
	}{
		{
			name: "successfully returns services",
			conn: &testDriver{
				rows: &testRows[dbmodel.Service]{
					data: []dbmodel.Service{
						{Name: "serviceA"},
						{Name: "serviceB"},
						{Name: "serviceC"},
					},
					scanFn: func(dest any, src dbmodel.Service) error {
						svc, ok := dest.(*dbmodel.Service)
						if !ok {
							return errors.New("dest is not *dbmodel.Service")
						}
						*svc = src
						return nil
					},
				},
			},
			expected: []string{"serviceA", "serviceB", "serviceC"},
		},
		{
			name:        "query error",
			conn:        &testDriver{err: assert.AnError},
			expectError: "failed to query services",
		},
		{
			name: "scan error",
			conn: &testDriver{
				rows: &testRows[dbmodel.Service]{
					data: []dbmodel.Service{
						{Name: "serviceA"},
						{Name: "serviceB"},
						{Name: "serviceC"},
					},
					scanFn: func(dest any, src dbmodel.Service) error {
						svc, ok := dest.(*dbmodel.Service)
						if !ok {
							return errors.New("dest is not *dbmodel.Service")
						}
						*svc = src
						return nil
					},
					scanErr: assert.AnError,
				},
			},
			expectError: "failed to scan row",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := NewReader(test.conn)

			result, err := reader.GetServices(context.Background())

			if test.expectError != "" {
				require.ErrorContains(t, err, test.expectError)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expected, result)
			}
		})
	}
}
