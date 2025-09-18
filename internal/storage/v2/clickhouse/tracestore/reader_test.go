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

var singleSpan = []*spanRow{
	&spanRow{
		id:                          "0000000000000001",
		traceID:                     traceID.String(),
		traceState:                  "state1",
		name:                        "GET /api/user",
		kind:                        "Server",
		startTime:                   now,
		statusCode:                  "Ok",
		statusMessage:               "success",
		rawDuration:                 1_000_000_000,
		boolAttributeKeys:           []string{"authenticated", "cache_hit"},
		boolAttributeValues:         []bool{true, false},
		doubleAttributeKeys:         []string{"response_time", "cpu_usage"},
		doubleAttributeValues:       []float64{0.123, 45.67},
		intAttributeKeys:            []string{"user_id", "request_size"},
		intAttributeValues:          []int64{12345, 1024},
		strAttributeKeys:            []string{"http.method", "http.url"},
		strAttributeValues:          []string{"GET", "/api/user"},
		complexAttributeKeys:        []string{"@bytes@request_body"},
		complexAttributeValues:      []string{"eyJuYW1lIjoidGVzdCJ9"},
		eventNames:                  []string{"login"},
		eventTimestamps:             []time.Time{now},
		eventBoolAttributeKeys:      [][]string{{"event.authenticated", "event.cached"}},
		eventBoolAttributeValues:    [][]bool{{true, false}},
		eventDoubleAttributeKeys:    [][]string{{"event.response_time"}},
		eventDoubleAttributeValues:  [][]float64{{0.001}},
		eventIntAttributeKeys:       [][]string{{"event.sequence"}},
		eventIntAttributeValues:     [][]int64{{1}},
		eventStrAttributeKeys:       [][]string{{"event.message"}},
		eventStrAttributeValues:     [][]string{{"user login successful"}},
		eventComplexAttributeKeys:   [][]string{{"@bytes@event.payload"}},
		eventComplexAttributeValues: [][]string{{"eyJ1c2VyX2lkIjoxMjM0NX0="}},
		linkTraceIDs:                []string{"00000000000000000000000000000002"},
		linkSpanIDs:                 []string{"0000000000000002"},
		linkTraceStates:             []string{"state2"},
		serviceName:                 "user-service",
		scopeName:                   "auth-scope",
		scopeVersion:                "v1.0.0",
	},
}

var multipleSpans = []*spanRow{
	&spanRow{
		id:                          "0000000000000001",
		traceID:                     traceID.String(),
		traceState:                  "state1",
		name:                        "GET /api/user",
		kind:                        "Server",
		startTime:                   now,
		statusCode:                  "Ok",
		statusMessage:               "success",
		rawDuration:                 1_000_000_000,
		boolAttributeKeys:           []string{"authenticated", "cache_hit"},
		boolAttributeValues:         []bool{true, false},
		doubleAttributeKeys:         []string{"response_time", "cpu_usage"},
		doubleAttributeValues:       []float64{0.123, 45.67},
		intAttributeKeys:            []string{"user_id", "request_size"},
		intAttributeValues:          []int64{12345, 1024},
		strAttributeKeys:            []string{"http.method", "http.url"},
		strAttributeValues:          []string{"GET", "/api/user"},
		complexAttributeKeys:        []string{"@bytes@request_body"},
		complexAttributeValues:      []string{"eyJuYW1lIjoidGVzdCJ9"},
		eventNames:                  []string{"login"},
		eventTimestamps:             []time.Time{now},
		eventBoolAttributeKeys:      [][]string{{"event.authenticated", "event.cached"}},
		eventBoolAttributeValues:    [][]bool{{true, false}},
		eventDoubleAttributeKeys:    [][]string{{"event.response_time"}},
		eventDoubleAttributeValues:  [][]float64{{0.001}},
		eventIntAttributeKeys:       [][]string{{"event.sequence"}},
		eventIntAttributeValues:     [][]int64{{1}},
		eventStrAttributeKeys:       [][]string{{"event.message"}},
		eventStrAttributeValues:     [][]string{{"user login successful"}},
		eventComplexAttributeKeys:   [][]string{{"@bytes@event.payload"}},
		eventComplexAttributeValues: [][]string{{"eyJ1c2VyX2lkIjoxMjM0NX0="}},
		linkTraceIDs:                []string{"00000000000000000000000000000002"},
		linkSpanIDs:                 []string{"0000000000000002"},
		linkTraceStates:             []string{"state2"},
		serviceName:                 "user-service",
		scopeName:                   "auth-scope",
		scopeVersion:                "v1.0.0",
	},
	&spanRow{
		id:                          "0000000000000003",
		traceID:                     traceID.String(),
		traceState:                  "state1",
		parentSpanID:                "0000000000000001",
		name:                        "SELECT /db/query",
		kind:                        "Client",
		startTime:                   now.Add(10 * time.Millisecond),
		statusCode:                  "Ok",
		statusMessage:               "success",
		rawDuration:                 500_000_000,
		boolAttributeKeys:           []string{"db.cached", "db.readonly"},
		boolAttributeValues:         []bool{false, true},
		doubleAttributeKeys:         []string{"db.latency", "db.connections"},
		doubleAttributeValues:       []float64{0.05, 5.0},
		intAttributeKeys:            []string{"db.rows_affected", "db.connection_id"},
		intAttributeValues:          []int64{150, 42},
		strAttributeKeys:            []string{"db.statement", "db.name"},
		strAttributeValues:          []string{"SELECT * FROM users", "userdb"},
		complexAttributeKeys:        []string{"@bytes@db.query_plan"},
		complexAttributeValues:      []string{"UExBTiBTRUxFQ1Q="},
		eventNames:                  []string{"query-start", "query-end"},
		eventTimestamps:             []time.Time{now.Add(10 * time.Millisecond), now.Add(510 * time.Millisecond)},
		eventBoolAttributeKeys:      [][]string{{"db.optimized", "db.indexed"}, {"db.cached", "db.successful"}},
		eventBoolAttributeValues:    [][]bool{{true, false}, {true, false}},
		eventDoubleAttributeKeys:    [][]string{{"db.query_time"}, {"db.result_time"}},
		eventDoubleAttributeValues:  [][]float64{{0.001}, {0.5}},
		eventIntAttributeKeys:       [][]string{{"db.connection_pool_size"}, {"db.result_count"}},
		eventIntAttributeValues:     [][]int64{{10}, {150}},
		eventStrAttributeKeys:       [][]string{{"db.event.type"}, {"db.event.status"}},
		eventStrAttributeValues:     [][]string{{"query_execution_start"}, {"query_execution_complete"}},
		eventComplexAttributeKeys:   [][]string{{"@bytes@db.query_metadata"}, {"@bytes@db.result_metadata"}},
		eventComplexAttributeValues: [][]string{{"eyJxdWVyeV9pZCI6MTIzfQ=="}, {"eyJyb3dfY291bnQiOjE1MH0="}},
		linkTraceIDs:                []string{},
		linkSpanIDs:                 []string{},
		linkTraceStates:             []string{},
		serviceName:                 "db-service",
		scopeName:                   "db-scope",
		scopeVersion:                "v1.0.0",
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

func scanSpanRowFn() func(dest any, src *spanRow) error {
	return func(dest any, src *spanRow) error {
		ptrs, ok := dest.([]any)
		if !ok {
			return fmt.Errorf("expected []any for dest, got %T", dest)
		}
		if len(ptrs) != 38 {
			return fmt.Errorf("expected 38 destination arguments, got %d", len(ptrs))
		}

		values := []any{
			&src.id,
			&src.traceID,
			&src.traceState,
			&src.parentSpanID,
			&src.name,
			&src.kind,
			&src.startTime,
			&src.statusCode,
			&src.statusMessage,
			&src.rawDuration,
			&src.boolAttributeKeys,
			&src.boolAttributeValues,
			&src.doubleAttributeKeys,
			&src.doubleAttributeValues,
			&src.intAttributeKeys,
			&src.intAttributeValues,
			&src.strAttributeKeys,
			&src.strAttributeValues,
			&src.complexAttributeKeys,
			&src.complexAttributeValues,
			&src.eventNames,
			&src.eventTimestamps,
			&src.eventBoolAttributeKeys,
			&src.eventBoolAttributeValues,
			&src.eventDoubleAttributeKeys,
			&src.eventDoubleAttributeValues,
			&src.eventIntAttributeKeys,
			&src.eventIntAttributeValues,
			&src.eventStrAttributeKeys,
			&src.eventStrAttributeValues,
			&src.eventComplexAttributeKeys,
			&src.eventComplexAttributeValues,
			&src.linkTraceIDs,
			&src.linkSpanIDs,
			&src.linkTraceStates,
			&src.serviceName,
			&src.scopeName,
			&src.scopeVersion,
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
		data     []*spanRow
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
				rows: &testRows[*spanRow]{
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
			requireTracesEqual(t, tt.data, traces)
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
				rows: &testRows[*spanRow]{
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
				rows: &testRows[*spanRow]{
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

	scanFn := func(dest any, src *spanRow) error {
		scanCalled++
		if scanCalled == 1 {
			return assert.AnError // simulate scan error on the first row
		}
		return scanSpanRowFn()(dest, src)
	}

	conn := &testDriver{
		t:             t,
		expectedQuery: sqlSelectSpansByTraceID,
		rows: &testRows[*spanRow]{
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
		requireTracesEqual(t, expected, trace)
	}
}

func TestGetTraces_YieldFalseOnSuccessStopsIteration(t *testing.T) {
	conn := &testDriver{
		t:             t,
		expectedQuery: sqlSelectSpansByTraceID,
		rows: &testRows[*spanRow]{
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
	requireTracesEqual(t, multipleSpans[0:1], gotTraces)
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
