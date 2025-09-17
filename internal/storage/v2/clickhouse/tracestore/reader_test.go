// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

var traceID = pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})

var now = time.Date(2025, 6, 14, 10, 0, 0, 0, time.UTC)

var singleSpan = []SpanRow{
	{
		ID:                          "0000000000000001",
		TraceID:                     traceID.String(),
		TraceState:                  "state1",
		Name:                        "GET /api/user",
		Kind:                        "Server",
		StartTime:                   now,
		StatusCode:                  "Ok",
		StatusMessage:               "success",
		RawDuration:                 1_000_000_000,
		BoolAttributeKeys:           []string{"authenticated", "cache_hit"},
		BoolAttributeValues:         []bool{true, false},
		DoubleAttributeKeys:         []string{"response_time", "cpu_usage"},
		DoubleAttributeValues:       []float64{0.123, 45.67},
		IntAttributeKeys:            []string{"user_id", "request_size"},
		IntAttributeValues:          []int64{12345, 1024},
		StrAttributeKeys:            []string{"http.method", "http.url"},
		StrAttributeValues:          []string{"GET", "/api/user"},
		ComplexAttributeKeys:        []string{"@bytes@request_body"},
		ComplexAttributeValues:      []string{"eyJuYW1lIjoidGVzdCJ9"},
		EventNames:                  []string{"login"},
		EventTimestamps:             []time.Time{now},
		EventBoolAttributeKeys:      [][]string{{"event.authenticated", "event.cached"}},
		EventBoolAttributeValues:    [][]bool{{true, false}},
		EventDoubleAttributeKeys:    [][]string{{"event.response_time"}},
		EventDoubleAttributeValues:  [][]float64{{0.001}},
		EventIntAttributeKeys:       [][]string{{"event.sequence"}},
		EventIntAttributeValues:     [][]int64{{1}},
		EventStrAttributeKeys:       [][]string{{"event.message"}},
		EventStrAttributeValues:     [][]string{{"user login successful"}},
		EventComplexAttributeKeys:   [][]string{{"@bytes@event.payload"}},
		EventComplexAttributeValues: [][]string{{"eyJ1c2VyX2lkIjoxMjM0NX0="}},
		LinkTraceIDs:                []string{"00000000000000000000000000000002"},
		LinkSpanIDs:                 []string{"0000000000000002"},
		LinkTraceStates:             []string{"state2"},
		ServiceName:                 "user-service",
		ScopeName:                   "auth-scope",
		ScopeVersion:                "v1.0.0",
	},
}

var multipleSpans = []SpanRow{
	{
		ID:                          "0000000000000001",
		TraceID:                     traceID.String(),
		TraceState:                  "state1",
		Name:                        "GET /api/user",
		Kind:                        "Server",
		StartTime:                   now,
		StatusCode:                  "Ok",
		StatusMessage:               "success",
		RawDuration:                 1_000_000_000,
		BoolAttributeKeys:           []string{"authenticated", "cache_hit"},
		BoolAttributeValues:         []bool{true, false},
		DoubleAttributeKeys:         []string{"response_time", "cpu_usage"},
		DoubleAttributeValues:       []float64{0.123, 45.67},
		IntAttributeKeys:            []string{"user_id", "request_size"},
		IntAttributeValues:          []int64{12345, 1024},
		StrAttributeKeys:            []string{"http.method", "http.url"},
		StrAttributeValues:          []string{"GET", "/api/user"},
		ComplexAttributeKeys:        []string{"@bytes@request_body"},
		ComplexAttributeValues:      []string{"eyJuYW1lIjoidGVzdCJ9"},
		EventNames:                  []string{"login"},
		EventTimestamps:             []time.Time{now},
		EventBoolAttributeKeys:      [][]string{{"event.authenticated", "event.cached"}},
		EventBoolAttributeValues:    [][]bool{{true, false}},
		EventDoubleAttributeKeys:    [][]string{{"event.response_time"}},
		EventDoubleAttributeValues:  [][]float64{{0.001}},
		EventIntAttributeKeys:       [][]string{{"event.sequence"}},
		EventIntAttributeValues:     [][]int64{{1}},
		EventStrAttributeKeys:       [][]string{{"event.message"}},
		EventStrAttributeValues:     [][]string{{"user login successful"}},
		EventComplexAttributeKeys:   [][]string{{"@bytes@event.payload"}},
		EventComplexAttributeValues: [][]string{{"eyJ1c2VyX2lkIjoxMjM0NX0="}},
		LinkTraceIDs:                []string{"00000000000000000000000000000002"},
		LinkSpanIDs:                 []string{"0000000000000002"},
		LinkTraceStates:             []string{"state2"},
		ServiceName:                 "user-service",
		ScopeName:                   "auth-scope",
		ScopeVersion:                "v1.0.0",
	},
	{
		ID:                          "0000000000000003",
		TraceID:                     traceID.String(),
		TraceState:                  "state1",
		ParentSpanID:                "0000000000000001",
		Name:                        "SELECT /db/query",
		Kind:                        "Client",
		StartTime:                   now.Add(10 * time.Millisecond),
		StatusCode:                  "Ok",
		StatusMessage:               "success",
		RawDuration:                 500_000_000,
		BoolAttributeKeys:           []string{"db.cached", "db.readonly"},
		BoolAttributeValues:         []bool{false, true},
		DoubleAttributeKeys:         []string{"db.latency", "db.connections"},
		DoubleAttributeValues:       []float64{0.05, 5.0},
		IntAttributeKeys:            []string{"db.rows_affected", "db.connection_id"},
		IntAttributeValues:          []int64{150, 42},
		StrAttributeKeys:            []string{"db.statement", "db.name"},
		StrAttributeValues:          []string{"SELECT * FROM users", "userdb"},
		ComplexAttributeKeys:        []string{"@bytes@db.query_plan"},
		ComplexAttributeValues:      []string{"UExBTiBTRUxFQ1Q="},
		EventNames:                  []string{"query-start", "query-end"},
		EventTimestamps:             []time.Time{now.Add(10 * time.Millisecond), now.Add(510 * time.Millisecond)},
		EventBoolAttributeKeys:      [][]string{{"db.optimized", "db.indexed"}, {"db.cached", "db.successful"}},
		EventBoolAttributeValues:    [][]bool{{true, false}, {true, false}},
		EventDoubleAttributeKeys:    [][]string{{"db.query_time"}, {"db.result_time"}},
		EventDoubleAttributeValues:  [][]float64{{0.001}, {0.5}},
		EventIntAttributeKeys:       [][]string{{"db.connection_pool_size"}, {"db.result_count"}},
		EventIntAttributeValues:     [][]int64{{10}, {150}},
		EventStrAttributeKeys:       [][]string{{"db.event.type"}, {"db.event.status"}},
		EventStrAttributeValues:     [][]string{{"query_execution_start"}, {"query_execution_complete"}},
		EventComplexAttributeKeys:   [][]string{{"@bytes@db.query_metadata"}, {"@bytes@db.result_metadata"}},
		EventComplexAttributeValues: [][]string{{"eyJxdWVyeV9pZCI6MTIzfQ=="}, {"eyJyb3dfY291bnQiOjE1MH0="}},
		LinkTraceIDs:                []string{},
		LinkSpanIDs:                 []string{},
		LinkTraceStates:             []string{},
		ServiceName:                 "db-service",
		ScopeName:                   "db-scope",
		ScopeVersion:                "v1.0.0",
	},
}

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

func scanSpanRowFn() func(dest any, src SpanRow) error {
	return func(dest any, src SpanRow) error {
		ptrs, ok := dest.([]any)
		if !ok {
			return fmt.Errorf("expected []any for dest, got %T", dest)
		}
		if len(ptrs) != 38 {
			return fmt.Errorf("expected 38 destination arguments, got %d", len(ptrs))
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
			&src.BoolAttributeKeys,
			&src.BoolAttributeValues,
			&src.DoubleAttributeKeys,
			&src.DoubleAttributeValues,
			&src.IntAttributeKeys,
			&src.IntAttributeValues,
			&src.StrAttributeKeys,
			&src.StrAttributeValues,
			&src.ComplexAttributeKeys,
			&src.ComplexAttributeValues,
			&src.EventNames,
			&src.EventTimestamps,
			&src.EventBoolAttributeKeys,
			&src.EventBoolAttributeValues,
			&src.EventDoubleAttributeKeys,
			&src.EventDoubleAttributeValues,
			&src.EventIntAttributeKeys,
			&src.EventIntAttributeValues,
			&src.EventStrAttributeKeys,
			&src.EventStrAttributeValues,
			&src.EventComplexAttributeKeys,
			&src.EventComplexAttributeValues,
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

func TestGetTraces_Success(t *testing.T) {
	tests := []struct {
		name     string
		data     []SpanRow
		expected []ptrace.Traces
	}{
		{
			name: "single span",
			data: singleSpan,
		},
		{
			name: "multiple spans",
			data: multipleSpans,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := &testDriver{
				t:             t,
				expectedQuery: sqlSelectSpansByTraceID,
				rows: &testRows[SpanRow]{
					data:   tt.data,
					scanFn: scanSpanRowFn(),
				},
			}

			reader := NewReader(conn)
			getTracesIter := reader.GetTraces(context.Background(), tracestore.GetTraceParams{
				TraceID: traceID,
			})
			traces, err := jiter.FlattenWithErrors(getTracesIter)

			require.NoError(t, err)
			RequireTracesEqual(t, tt.data, traces)
		})
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
				rows: &testRows[SpanRow]{
					data:    singleSpan,
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
				rows: &testRows[SpanRow]{
					data:     singleSpan,
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
				TraceID: traceID,
			})
			_, err := jiter.FlattenWithErrors(iter)
			require.ErrorContains(t, err, test.expectedErr)
		})
	}
}

func TestGetTraces_ScanErrorContinues(t *testing.T) {
	scanCalled := 0

	scanFn := func(dest any, src SpanRow) error {
		scanCalled++
		if scanCalled == 1 {
			return assert.AnError // simulate scan error on the first row
		}
		return scanSpanRowFn()(dest, src)
	}

	conn := &testDriver{
		t:             t,
		expectedQuery: sqlSelectSpansByTraceID,
		rows: &testRows[SpanRow]{
			data:   multipleSpans,
			scanFn: scanFn,
		},
	}

	reader := NewReader(conn)
	getTracesIter := reader.GetTraces(context.Background(), tracestore.GetTraceParams{
		TraceID: traceID,
	})

	expected := multipleSpans[1:] // skip the first span which caused the error
	for trace, err := range getTracesIter {
		if err != nil {
			require.ErrorIs(t, err, assert.AnError)
			continue
		}
		RequireTracesEqual(t, expected, trace)
	}
}

func TestGetTraces_YieldFalseOnSuccessStopsIteration(t *testing.T) {
	conn := &testDriver{
		t:             t,
		expectedQuery: sqlSelectSpansByTraceID,
		rows: &testRows[SpanRow]{
			data:   multipleSpans,
			scanFn: scanSpanRowFn(),
		},
	}

	reader := NewReader(conn)
	getTracesIter := reader.GetTraces(context.Background(), tracestore.GetTraceParams{
		TraceID: traceID,
	})

	var gotTraces []ptrace.Traces
	getTracesIter(func(traces []ptrace.Traces, err error) bool {
		require.NoError(t, err)
		gotTraces = append(gotTraces, traces...)
		return false // stop iteration after the first span
	})

	require.Len(t, gotTraces, 1)
	RequireTracesEqual(t, multipleSpans[0:1], gotTraces)
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
