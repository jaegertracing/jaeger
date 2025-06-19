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
	"go.opentelemetry.io/collector/pdata/ptrace"

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

func scanSpanRowFn() func(dest any, src testdata.SpanRow) error {
	return func(dest any, src testdata.SpanRow) error {
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
	}
}

func TestGetTraces_ErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		driver      *testDriver
		expectedErr string
	}{
		{
			name: "QueryError",
			driver: &testDriver{
				t:             t,
				expectedQuery: sqlSelectSpansByTraceID,
				err:           assert.AnError,
			},
			expectedErr: "failed to query trace",
		},
		{
			name: "ScanError",
			driver: &testDriver{
				t:             t,
				expectedQuery: sqlSelectSpansByTraceID,
				rows: &testRows[testdata.SpanRow]{
					data:    testdata.SingleSpan,
					scanErr: assert.AnError,
				},
			},
			expectedErr: "failed to scan span row",
		},
		{
			name: "CloseError",
			driver: &testDriver{
				t:             t,
				expectedQuery: sqlSelectSpansByTraceID,
				rows: &testRows[testdata.SpanRow]{
					data:     testdata.SingleSpan,
					scanFn:   scanSpanRowFn(),
					closeErr: assert.AnError,
				},
			},
			expectedErr: "failed to close rows",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := NewReader(test.driver)
			iter := reader.GetTraces(context.Background(), tracestore.GetTraceParams{
				TraceID: testdata.TraceID,
			})
			_, err := jiter.FlattenWithErrors(iter)
			require.ErrorContains(t, err, test.expectedErr)
		})
	}
}

func TestGetTraces_SingleSpan(t *testing.T) {
	conn := &testDriver{
		t:             t,
		expectedQuery: sqlSelectSpansByTraceID,
		rows: &testRows[testdata.SpanRow]{
			data:   testdata.SingleSpan,
			scanFn: scanSpanRowFn(),
		},
	}

	reader := NewReader(conn)
	getTracesIter := reader.GetTraces(context.Background(), tracestore.GetTraceParams{
		TraceID: testdata.TraceID,
	})
	traces, err := jiter.FlattenWithErrors(getTracesIter)

	require.NoError(t, err)
	testdata.RequireTracesEqual(t, testdata.SingleSpan, traces)
}

func TestGetTraces_MultipleSpans(t *testing.T) {
	conn := &testDriver{
		t:             t,
		expectedQuery: sqlSelectSpansByTraceID,
		rows: &testRows[testdata.SpanRow]{
			data:   testdata.MultipleSpans,
			scanFn: scanSpanRowFn(),
		},
	}

	reader := NewReader(conn)
	getTracesIter := reader.GetTraces(context.Background(), tracestore.GetTraceParams{
		TraceID: testdata.TraceID,
	})
	traces, err := jiter.FlattenWithErrors(getTracesIter)

	require.NoError(t, err)
	testdata.RequireTracesEqual(t, testdata.MultipleSpans, traces)
}

func TestGetTraces_ScanErrorContinues(t *testing.T) {
	scanCalled := 0

	conn := &testDriver{
		t:             t,
		expectedQuery: sqlSelectSpansByTraceID,
		rows: &testRows[testdata.SpanRow]{
			data: testdata.MultipleSpans,
			scanFn: func(dest any, src testdata.SpanRow) error {
				scanCalled++
				if scanCalled == 1 {
					return assert.AnError // simulate scan error on the first row
				}
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
		TraceID: testdata.TraceID,
	})

	expected := testdata.MultipleSpans[1:] // skip the first span which caused the error
	for trace, err := range getTracesIter {
		if err != nil {
			require.ErrorIs(t, err, assert.AnError)
			continue
		}
		testdata.RequireTracesEqual(t, expected, trace)
	}
}

func TestGetTraces_YieldFalseOnSuccessStopsIteration(t *testing.T) {
	conn := &testDriver{
		t:             t,
		expectedQuery: sqlSelectSpansByTraceID,
		rows: &testRows[testdata.SpanRow]{
			data:   testdata.MultipleSpans,
			scanFn: scanSpanRowFn(),
		},
	}

	reader := NewReader(conn)
	getTracesIter := reader.GetTraces(context.Background(), tracestore.GetTraceParams{
		TraceID: testdata.TraceID,
	})

	var gotTraces []ptrace.Traces
	getTracesIter(func(traces []ptrace.Traces, err error) bool {
		require.NoError(t, err)
		gotTraces = append(gotTraces, traces...)
		return false // stop iteration after the first span
	})

	require.Len(t, gotTraces, 1)
	testdata.RequireTracesEqual(t, testdata.MultipleSpans[0:1], gotTraces)
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
