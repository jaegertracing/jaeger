// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/testdata"
)

type testDriver struct {
	driver.Conn

	t             *testing.T
	rows          driver.Rows
	expectedQuery string
	err           error
}

func (t *testDriver) Query(_ context.Context, query string, _ ...any) (driver.Rows, error) {
	require.Equal(t.t, t.expectedQuery, query)
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

func (f *testRows[T]) Scan(dest ...any) error {
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

func TestGetTraces_QueryError(t *testing.T) {
	conn := &testDriver{
		t:             t,
		expectedQuery: sqlSelectSpansByTraceID,
		err:           assert.AnError,
	}

	reader := NewReader(conn)
	getTracesIter := reader.GetTraces(context.Background(), tracestore.GetTraceParams{
		TraceID: pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}),
	})
	_, err := jiter.FlattenWithErrors(getTracesIter)
	require.ErrorContains(t, err, "failed to query trace")
	require.ErrorIs(t, err, assert.AnError)
}

func TestGetTraces_ScanError(t *testing.T) {
	conn := &testDriver{
		t:             t,
		expectedQuery: sqlSelectSpansByTraceID,
		rows: &testRows[testdata.SpanRow]{
			data:    []testdata.SpanRow{testdata.SingleSpan},
			scanErr: assert.AnError,
		},
	}

	reader := NewReader(conn)
	getTracesIter := reader.GetTraces(context.Background(), tracestore.GetTraceParams{
		TraceID: pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}),
	})
	_, err := jiter.FlattenWithErrors(getTracesIter)
	require.ErrorContains(t, err, "failed to scan span row")
	require.ErrorIs(t, err, assert.AnError)
}

func TestGetTraces_SingleTrace(t *testing.T) {
	conn := &testDriver{
		t:             t,
		expectedQuery: sqlSelectSpansByTraceID,
		rows: &testRows[testdata.SpanRow]{
			data: []testdata.SpanRow{testdata.SingleSpan},
			scanFn: func(dest any, src testdata.SpanRow) error {
				ptrs, ok := dest.([]any)
				if !ok {
					return fmt.Errorf("expected []any for dest, got %T", dest)
				}
				if len(ptrs) != 18 {
					return fmt.Errorf("expected 18 destination arguments, got %d", len(ptrs))
				}

				values := []any{
					&src.ID,
					&src.TraceID,
					&src.TraceState,
					&src.ParentSpanID,
					&src.Name,
					&src.Kind,
					&src.StartTime,
					&src.StatusCode,
					&src.StatusMessage,
					&src.RawDuration,
					&src.EventNames,
					&src.EventTimestamps,
					&src.LinkTraceIDs,
					&src.LinkSpanIDs,
					&src.LinkTraceStates,
					&src.ServiceName,
					&src.ScopeName,
					&src.ScopeVersion,
				}

				for i := range ptrs {
					reflect.ValueOf(ptrs[i]).Elem().Set(reflect.ValueOf(values[i]).Elem())
				}
				return nil
			},
		},
	}

	reader := NewReader(conn)
	getTracesIter := reader.GetTraces(context.Background(), tracestore.GetTraceParams{
		TraceID: pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}),
	})
	traces, err := jiter.FlattenWithErrors(getTracesIter)

	require.NoError(t, err)
	require.Len(t, traces, 1)

	resourceSpans := traces[0].ResourceSpans()
	require.Equal(t, 1, resourceSpans.Len())

	scopeSpans := resourceSpans.At(0).ScopeSpans()
	require.Equal(t, 1, scopeSpans.Len())
	testdata.RequireScopeEqual(t, testdata.SingleSpan, scopeSpans.At(0).Scope())

	spans := scopeSpans.At(0).Spans()
	require.Equal(t, 1, spans.Len())

	testdata.RequireSpanEqual(t, testdata.SingleSpan, spans.At(0))
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
				t:             t,
				expectedQuery: sqlSelectAllServices,
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
			name: "query error",
			conn: &testDriver{
				t:             t,
				expectedQuery: sqlSelectAllServices,
				err:           assert.AnError,
			},
			expectError: "failed to query services",
		},
		{
			name: "scan error",
			conn: &testDriver{
				t:             t,
				expectedQuery: sqlSelectAllServices,
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

func TestGetOperations(t *testing.T) {
	tests := []struct {
		name        string
		conn        *testDriver
		query       tracestore.OperationQueryParams
		expected    []tracestore.Operation
		expectError string
	}{
		{
			name: "successfully returns operations for all kinds",
			conn: &testDriver{
				t:             t,
				expectedQuery: sqlSelectOperationsAllKinds,
				rows: &testRows[dbmodel.Operation]{
					data: []dbmodel.Operation{
						{Name: "operationA"},
						{Name: "operationB"},
						{Name: "operationC"},
					},
					scanFn: func(dest any, src dbmodel.Operation) error {
						svc, ok := dest.(*dbmodel.Operation)
						if !ok {
							return errors.New("dest is not *dbmodel.Operation")
						}
						*svc = src
						return nil
					},
				},
			},
			query: tracestore.OperationQueryParams{
				ServiceName: "serviceA",
			},
			expected: []tracestore.Operation{
				{
					Name: "operationA",
				},
				{
					Name: "operationB",
				},
				{
					Name: "operationC",
				},
			},
		},
		{
			name: "successfully returns operations by kind",
			conn: &testDriver{
				t:             t,
				expectedQuery: sqlSelectOperationsByKind,
				rows: &testRows[dbmodel.Operation]{
					data: []dbmodel.Operation{
						{Name: "operationA", SpanKind: "server"},
						{Name: "operationB", SpanKind: "server"},
						{Name: "operationC", SpanKind: "server"},
					},
					scanFn: func(dest any, src dbmodel.Operation) error {
						svc, ok := dest.(*dbmodel.Operation)
						if !ok {
							return errors.New("dest is not *dbmodel.Operation")
						}
						*svc = src
						return nil
					},
				},
			},
			query: tracestore.OperationQueryParams{
				ServiceName: "serviceA",
				SpanKind:    "server",
			},
			expected: []tracestore.Operation{
				{
					Name:     "operationA",
					SpanKind: "server",
				},
				{
					Name:     "operationB",
					SpanKind: "server",
				},
				{
					Name:     "operationC",
					SpanKind: "server",
				},
			},
		},
		{
			name: "query error",
			conn: &testDriver{
				t:             t,
				expectedQuery: sqlSelectOperationsAllKinds,
				err:           assert.AnError,
			},
			expectError: "failed to query operations",
		},
		{
			name: "scan error",
			conn: &testDriver{
				t:             t,
				expectedQuery: sqlSelectOperationsAllKinds,
				rows: &testRows[dbmodel.Operation]{
					data: []dbmodel.Operation{
						{Name: "operationA"},
						{Name: "operationB"},
						{Name: "operationC"},
					},
					scanFn: func(dest any, src dbmodel.Operation) error {
						svc, ok := dest.(*dbmodel.Operation)
						if !ok {
							return errors.New("dest is not *dbmodel.Operation")
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

			result, err := reader.GetOperations(context.Background(), test.query)

			if test.expectError != "" {
				require.ErrorContains(t, err, test.expectError)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.expected, result)
			}
		})
	}
}
