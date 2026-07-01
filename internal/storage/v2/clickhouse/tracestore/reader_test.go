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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

var (
	testReaderConfig = ReaderConfig{
		DefaultSearchDepth:            100,
		MaxSearchDepth:                1000,
		AttributeMetadataCacheMaxSize: 1000,
	}
	testTraceIDsData = [][]any{
		{
			traceIDHex1,
			now.Add(-1 * time.Hour),
			now,
		},
		{
			traceIDHex2,
			time.Time{},
			time.Time{},
		},
	}
	testAttributeMetadata = []dbmodel.AttributeMetadata{
		{AttributeKey: "span.flag", Type: "bool", Level: "span"},
		{AttributeKey: "resource.latency", Type: "double", Level: "resource"},
		{AttributeKey: "scope.attempt", Type: "int", Level: "scope"},
		{AttributeKey: "http.method", Type: "str", Level: "span"},
		{AttributeKey: "http.method", Type: "int", Level: "span"},
		{AttributeKey: "resource.checksum", Type: "bytes", Level: "resource"},
		{AttributeKey: "metadata", Type: "map", Level: "span"},
		{AttributeKey: "tags", Type: "slice", Level: "span"},
		{AttributeKey: "event.attr", Type: "str", Level: "event"},
	}
)

func buildTestAttributes() pcommon.Map {
	attrs := pcommon.NewMap()
	attrs.PutBool("login_successful", true)
	attrs.PutDouble("response_time", 0.123)
	attrs.PutInt("attempt_count", 1)
	b := attrs.PutEmptyBytes("file.checksum")
	s := attrs.PutEmptySlice("http.headers")
	m := attrs.PutEmptyMap("http.cookies")

	b.FromRaw([]byte{0x12, 0x34, 0x56, 0x78})
	s.AppendEmpty().SetStr("header1: value1")
	m.PutStr("session_id", "abc123")

	// these attributes will require type lookup from attribute_metadata
	attrs.PutStr("no.metadata", "nonexistent") // no metadata entry
	attrs.PutStr("http.method", "GET")
	attrs.PutStr("span.flag", "true")
	attrs.PutStr("resource.latency", "0.5")
	attrs.PutStr("scope.attempt", "7")
	attrs.PutStr("resource.checksum", "EjRWeA==")
	attrs.PutStr("metadata", "{\"kvlistValue\":{\"values\":[{\"key\":\"key\",\"value\":{\"stringValue\":\"value\"}}]}}")
	attrs.PutStr("tags", "{\"arrayValue\":{\"values\":[{\"intValue\":\"1\"},{\"intValue\":\"2\"},{\"intValue\":\"3\"}]}}")
	attrs.PutStr("event.attr", "event-value")

	return attrs
}

func scanSpanRowFn() func(dest any, src *dbmodel.SpanRow) error {
	return func(dest any, src *dbmodel.SpanRow) error {
		ptrs, ok := dest.([]any)
		if !ok {
			return fmt.Errorf("expected []any for dest, got %T", dest)
		}
		if len(ptrs) != 68 {
			return fmt.Errorf("expected 68 destination arguments, got %d", len(ptrs))
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
			&src.Duration,
			&src.Attributes.BoolKeys,
			&src.Attributes.BoolValues,
			&src.Attributes.DoubleKeys,
			&src.Attributes.DoubleValues,
			&src.Attributes.IntKeys,
			&src.Attributes.IntValues,
			&src.Attributes.StrKeys,
			&src.Attributes.StrValues,
			&src.Attributes.ComplexKeys,
			&src.Attributes.ComplexValues,
			&src.EventNames,
			&src.EventTimestamps,
			&src.EventAttributes.BoolKeys,
			&src.EventAttributes.BoolValues,
			&src.EventAttributes.DoubleKeys,
			&src.EventAttributes.DoubleValues,
			&src.EventAttributes.IntKeys,
			&src.EventAttributes.IntValues,
			&src.EventAttributes.StrKeys,
			&src.EventAttributes.StrValues,
			&src.EventAttributes.ComplexKeys,
			&src.EventAttributes.ComplexValues,
			&src.LinkTraceIDs,
			&src.LinkSpanIDs,
			&src.LinkTraceStates,
			&src.LinkAttributes.BoolKeys,
			&src.LinkAttributes.BoolValues,
			&src.LinkAttributes.DoubleKeys,
			&src.LinkAttributes.DoubleValues,
			&src.LinkAttributes.IntKeys,
			&src.LinkAttributes.IntValues,
			&src.LinkAttributes.StrKeys,
			&src.LinkAttributes.StrValues,
			&src.LinkAttributes.ComplexKeys,
			&src.LinkAttributes.ComplexValues,
			&src.ServiceName,
			&src.ResourceAttributes.BoolKeys,
			&src.ResourceAttributes.BoolValues,
			&src.ResourceAttributes.DoubleKeys,
			&src.ResourceAttributes.DoubleValues,
			&src.ResourceAttributes.IntKeys,
			&src.ResourceAttributes.IntValues,
			&src.ResourceAttributes.StrKeys,
			&src.ResourceAttributes.StrValues,
			&src.ResourceAttributes.ComplexKeys,
			&src.ResourceAttributes.ComplexValues,
			&src.ScopeName,
			&src.ScopeVersion,
			&src.ScopeAttributes.BoolKeys,
			&src.ScopeAttributes.BoolValues,
			&src.ScopeAttributes.DoubleKeys,
			&src.ScopeAttributes.DoubleValues,
			&src.ScopeAttributes.IntKeys,
			&src.ScopeAttributes.IntValues,
			&src.ScopeAttributes.StrKeys,
			&src.ScopeAttributes.StrValues,
			&src.ScopeAttributes.ComplexKeys,
			&src.ScopeAttributes.ComplexValues,
		}

		for i := range ptrs {
			reflect.ValueOf(ptrs[i]).Elem().Set(reflect.ValueOf(values[i]).Elem())
		}
		return nil
	}
}

func scanAttributeMetadataFn() func(dest any, src dbmodel.AttributeMetadata) error {
	return func(dest any, src dbmodel.AttributeMetadata) error {
		ptr, ok := dest.(*dbmodel.AttributeMetadata)
		if !ok {
			return fmt.Errorf("expected *dbmodel.AttributeMetadata for dest, got %T", dest)
		}
		*ptr = src
		return nil
	}
}

func scanTraceIDFn() func(dest any, src []any) error {
	return func(dest any, src []any) error {
		ptrs, ok := dest.([]any)
		if !ok {
			return fmt.Errorf("expected []any for dest, got %T", dest)
		}
		if len(ptrs) != 3 {
			fmt.Println(src)
			return fmt.Errorf("expected 3 destination arguments, got %d", len(ptrs))
		}

		ptr, ok := ptrs[0].(*string)
		if !ok {
			return fmt.Errorf("expected *string for dest[0], got %T", ptrs[0])
		}

		startPtr, ok := ptrs[1].(*time.Time)
		if !ok {
			return fmt.Errorf("expected *time.Time for dest[1], got %T", ptrs[1])
		}

		endPtr, ok := ptrs[2].(*time.Time)
		if !ok {
			return fmt.Errorf("expected *time.Time for dest[2], got %T", ptrs[2])
		}

		*ptr = src[0].(string)
		*startPtr = src[1].(time.Time)
		*endPtr = src[2].(time.Time)
		return nil
	}
}

func TestGetTraces_Success(t *testing.T) {
	tests := []struct {
		name   string
		params tracestore.GetTraceParams
		data   []*dbmodel.SpanRow
	}{
		{
			name: "single span",
			params: tracestore.GetTraceParams{
				TraceID: traceID,
			},
			data: singleSpan,
		},
		{
			name: "multiple spans",
			params: tracestore.GetTraceParams{
				TraceID: traceID,
			},
			data: multipleSpans,
		},
		{
			name: "with time range",
			params: tracestore.GetTraceParams{
				TraceID: traceID,
				Start:   now.Add(-1 * time.Hour),
				End:     now,
			},
			data: singleSpan,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectSpansByTraceID: {
						Rows: &clickhousetest.Rows[*dbmodel.SpanRow]{
							Data:   tt.data,
							ScanFn: scanSpanRowFn(),
						},
						Err: nil,
					},
				},
			}

			reader := NewReader(conn, testReaderConfig)
			getTracesIter := reader.GetTraces(context.Background(), tt.params)
			traces, err := jiter.FlattenWithErrors(getTracesIter)

			require.NoError(t, err)
			require.Len(t, conn.RecordedQueries, 1)
			verifyQuerySnapshot(t, conn.RecordedQueries...)
			requireTracesEqual(t, tt.data, traces)
		})
	}
}

func TestGetTraces_ErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		driver      *clickhousetest.Driver
		expectedErr string
	}{
		{
			name: "QueryError",
			driver: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectSpansByTraceID: {
						Rows: nil,
						Err:  assert.AnError,
					},
				},
			},
			expectedErr: "failed to query trace",
		},
		{
			name: "ScanError",
			driver: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectSpansByTraceID: {
						Rows: &clickhousetest.Rows[*dbmodel.SpanRow]{
							Data:    singleSpan,
							ScanErr: assert.AnError,
						},
						Err: nil,
					},
				},
			},
			expectedErr: "failed to scan span row",
		},
		{
			name: "CloseError",
			driver: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectSpansByTraceID: {
						Rows: &clickhousetest.Rows[*dbmodel.SpanRow]{
							Data:     singleSpan,
							ScanFn:   scanSpanRowFn(),
							CloseErr: assert.AnError,
						},
						Err: nil,
					},
				},
			},
			expectedErr: "failed to close rows",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := NewReader(test.driver, testReaderConfig)
			iter := reader.GetTraces(context.Background(), tracestore.GetTraceParams{
				TraceID: traceID,
			})
			_, err := jiter.FlattenWithErrors(iter)
			require.ErrorContains(t, err, test.expectedErr)
		})
	}
}

func TestGetTraces_RowsError(t *testing.T) {
	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectSpansByTraceID: {
				Rows: &clickhousetest.Rows[*dbmodel.SpanRow]{
					RowsErr: assert.AnError,
				},
			},
		},
	}
	reader := NewReader(conn, testReaderConfig)
	iter := reader.GetTraces(context.Background(), tracestore.GetTraceParams{
		TraceID: traceID,
	})
	_, err := jiter.FlattenWithErrors(iter)
	require.ErrorContains(t, err, "failed to read span rows")
}

func TestGetTraces_ScanErrorStopsIteration(t *testing.T) {
	scanCalled := 0

	scanFn := func(dest any, src *dbmodel.SpanRow) error {
		scanCalled++
		if scanCalled == 1 {
			return assert.AnError // simulate scan error on the first row
		}
		return scanSpanRowFn()(dest, src)
	}

	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectSpansByTraceID: {
				Rows: &clickhousetest.Rows[*dbmodel.SpanRow]{
					Data:   multipleSpans,
					ScanFn: scanFn,
				},
				Err: nil,
			},
		},
	}

	reader := NewReader(conn, testReaderConfig)
	iter := reader.GetTraces(context.Background(), tracestore.GetTraceParams{
		TraceID: traceID,
	})
	_, err := jiter.FlattenWithErrors(iter)
	require.ErrorContains(t, err, "failed to scan span row")
}

func TestGetTraces_YieldFalseOnSuccessStopsIteration(t *testing.T) {
	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectSpansByTraceID: {
				Rows: &clickhousetest.Rows[*dbmodel.SpanRow]{
					Data:   multipleSpans,
					ScanFn: scanSpanRowFn(),
				},
				Err: nil,
			},
		},
	}

	reader := NewReader(conn, testReaderConfig)
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

func TestGetTraces_YieldFalseSkipsRowsError(t *testing.T) {
	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectSpansByTraceID: {
				Rows: &clickhousetest.Rows[*dbmodel.SpanRow]{
					Data:    multipleSpans,
					ScanFn:  scanSpanRowFn(),
					RowsErr: assert.AnError,
				},
				Err: nil,
			},
		},
	}

	reader := NewReader(conn, testReaderConfig)
	getTracesIter := reader.GetTraces(context.Background(), tracestore.GetTraceParams{
		TraceID: traceID,
	})

	called := 0
	getTracesIter(func(_ []ptrace.Traces, err error) bool {
		called++
		require.NoError(t, err)
		return false
	})

	require.Equal(t, 1, called)
}

func TestGetServices(t *testing.T) {
	tests := []struct {
		name        string
		conn        *clickhousetest.Driver
		expected    []string
		expectError string
	}{
		{
			name: "successfully returns services",
			conn: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectServices: {
						Rows: &clickhousetest.Rows[dbmodel.Service]{
							Data: []dbmodel.Service{
								{Name: "serviceA"},
								{Name: "serviceB"},
								{Name: "serviceC"},
							},
							ScanFn: func(dest any, src dbmodel.Service) error {
								svc, ok := dest.(*dbmodel.Service)
								if !ok {
									return errors.New("dest is not *dbmodel.Service")
								}
								*svc = src
								return nil
							},
						},
						Err: nil,
					},
				},
			},
			expected: []string{"serviceA", "serviceB", "serviceC"},
		},
		{
			name: "query error",
			conn: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectServices: {
						Rows: nil,
						Err:  assert.AnError,
					},
				},
			},
			expectError: "failed to query services",
		},
		{
			name: "scan error",
			conn: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectServices: {
						Rows: &clickhousetest.Rows[dbmodel.Service]{
							Data: []dbmodel.Service{
								{Name: "serviceA"},
								{Name: "serviceB"},
								{Name: "serviceC"},
							},
							ScanErr: assert.AnError,
						},
						Err: nil,
					},
				},
			},
			expectError: "failed to scan row",
		},
		{
			name: "rows error",
			conn: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectServices: {
						Rows: &clickhousetest.Rows[dbmodel.Service]{
							RowsErr: assert.AnError,
						},
					},
				},
			},
			expectError: "failed to read service rows",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := NewReader(test.conn, testReaderConfig)

			result, err := reader.GetServices(context.Background())

			if test.expectError != "" {
				require.ErrorContains(t, err, test.expectError)
			} else {
				require.NoError(t, err)
				require.Len(t, test.conn.RecordedQueries, 1)
				verifyQuerySnapshot(t, test.conn.RecordedQueries...)
				require.Equal(t, test.expected, result)
			}
		})
	}
}

func TestGetOperations(t *testing.T) {
	tests := []struct {
		name        string
		conn        *clickhousetest.Driver
		query       tracestore.OperationQueryParams
		expected    []tracestore.Operation
		expectError string
	}{
		{
			name: "successfully returns operations for all kinds",
			conn: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectOperationsAllKinds: {
						Rows: &clickhousetest.Rows[dbmodel.Operation]{
							Data: []dbmodel.Operation{
								{Name: "operationA"},
								{Name: "operationB"},
								{Name: "operationC"},
							},
							ScanFn: func(dest any, src dbmodel.Operation) error {
								svc, ok := dest.(*dbmodel.Operation)
								if !ok {
									return errors.New("dest is not *dbmodel.Operation")
								}
								*svc = src
								return nil
							},
						},
						Err: nil,
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
			conn: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectOperationsByKind: {
						Rows: &clickhousetest.Rows[dbmodel.Operation]{
							Data: []dbmodel.Operation{
								{Name: "operationA", SpanKind: "server"},
								{Name: "operationB", SpanKind: "server"},
								{Name: "operationC", SpanKind: "server"},
							},
							ScanFn: func(dest any, src dbmodel.Operation) error {
								svc, ok := dest.(*dbmodel.Operation)
								if !ok {
									return errors.New("dest is not *dbmodel.Operation")
								}
								*svc = src
								return nil
							},
						},
						Err: nil,
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
			conn: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectOperationsAllKinds: {
						Rows: nil,
						Err:  assert.AnError,
					},
				},
			},
			expectError: "failed to query operations",
		},
		{
			name: "scan error",
			conn: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectOperationsAllKinds: {
						Rows: &clickhousetest.Rows[dbmodel.Operation]{
							Data: []dbmodel.Operation{
								{Name: "operationA"},
								{Name: "operationB"},
								{Name: "operationC"},
							},
							ScanErr: assert.AnError,
						},
						Err: nil,
					},
				},
			},
			expectError: "failed to scan row",
		},
		{
			name: "rows error",
			conn: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectOperationsAllKinds: {
						Rows: &clickhousetest.Rows[dbmodel.Operation]{
							RowsErr: assert.AnError,
						},
					},
				},
			},
			expectError: "failed to read operation rows",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := NewReader(test.conn, testReaderConfig)

			result, err := reader.GetOperations(context.Background(), test.query)

			if test.expectError != "" {
				require.ErrorContains(t, err, test.expectError)
			} else {
				require.NoError(t, err)
				require.Len(t, test.conn.RecordedQueries, 1)
				verifyQuerySnapshot(t, test.conn.RecordedQueries...)
				require.Equal(t, test.expected, result)
			}
		})
	}
}

func TestFindTraces_Success(t *testing.T) {
	tests := []struct {
		name string
		data []*dbmodel.SpanRow
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
			conn := &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectSpansQuery: {
						Rows: &clickhousetest.Rows[*dbmodel.SpanRow]{
							Data:   tt.data,
							ScanFn: scanSpanRowFn(),
						},
						Err: nil,
					},
				},
			}

			reader := NewReader(conn, testReaderConfig)
			findTracesIter := reader.FindTraces(context.Background(), tracestore.TraceQueryParams{
				Attributes: pcommon.NewMap(),
			})
			traces, err := jiter.FlattenWithErrors(findTracesIter)

			require.NoError(t, err)
			require.Len(t, conn.RecordedQueries, 1)
			verifyQuerySnapshot(t, conn.RecordedQueries...)
			requireTracesEqual(t, tt.data, traces)
		})
	}
}

func TestFindTraces_WithFilters(t *testing.T) {
	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectAttributeMetadata: {
				Rows: &clickhousetest.Rows[dbmodel.AttributeMetadata]{
					Data:   testAttributeMetadata,
					ScanFn: scanAttributeMetadataFn(),
				},
			},
			sql.SelectSpansQuery: {
				Rows: &clickhousetest.Rows[*dbmodel.SpanRow]{
					Data:   multipleSpans,
					ScanFn: scanSpanRowFn(),
				},
				Err: nil,
			},
		},
	}

	reader := NewReader(conn, testReaderConfig)
	attributes := buildTestAttributes()

	iter := reader.FindTraces(context.Background(), tracestore.TraceQueryParams{
		ServiceName:   "serviceA",
		OperationName: "operationA",
		DurationMin:   1 * time.Nanosecond,
		DurationMax:   1 * time.Second,
		StartTimeMin:  now.Add(-1 * time.Hour),
		StartTimeMax:  now,
		Attributes:    attributes,
		SearchDepth:   5,
	})
	traces, err := jiter.FlattenWithErrors(iter)
	require.NoError(t, err)
	require.Len(t, conn.RecordedQueries, 2)
	verifyQuerySnapshot(t, conn.RecordedQueries...)
	requireTracesEqual(t, multipleSpans, traces)
}

func TestFindTraces_SearchDepthExceedsMax(t *testing.T) {
	driver := &clickhousetest.Driver{}
	reader := NewReader(driver, testReaderConfig)
	iter := reader.FindTraces(context.Background(), tracestore.TraceQueryParams{
		SearchDepth: 10000,
		Attributes:  pcommon.NewMap(),
	})
	_, err := jiter.FlattenWithErrors(iter)
	require.ErrorContains(t, err, "search depth 10000 exceeds maximum allowed 1000")
}

func TestFindTraces_YieldFalseOnSuccessStopsIteration(t *testing.T) {
	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectSpansQuery: {
				Rows: &clickhousetest.Rows[*dbmodel.SpanRow]{
					Data:   multipleSpans,
					ScanFn: scanSpanRowFn(),
				},
				Err: nil,
			},
		},
	}

	reader := NewReader(conn, testReaderConfig)
	findTracesIter := reader.FindTraces(context.Background(), tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
	})

	var gotTraces []ptrace.Traces
	findTracesIter(func(traces []ptrace.Traces, err error) bool {
		require.NoError(t, err)
		gotTraces = append(gotTraces, traces...)
		return false // stop iteration after the first span
	})

	require.Len(t, gotTraces, 1)
	requireTracesEqual(t, multipleSpans[0:1], gotTraces)
}

func TestFindTraces_YieldFalseSkipsRowsError(t *testing.T) {
	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectSpansQuery: {
				Rows: &clickhousetest.Rows[*dbmodel.SpanRow]{
					Data:    multipleSpans,
					ScanFn:  scanSpanRowFn(),
					RowsErr: assert.AnError,
				},
				Err: nil,
			},
		},
	}

	reader := NewReader(conn, testReaderConfig)
	findTracesIter := reader.FindTraces(context.Background(), tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
	})

	called := 0
	findTracesIter(func(_ []ptrace.Traces, err error) bool {
		called++
		require.NoError(t, err)
		return false
	})

	require.Equal(t, 1, called)
}

func TestFindTraces_ScanErrorStopsIteration(t *testing.T) {
	scanCalled := 0

	scanFn := func(dest any, src *dbmodel.SpanRow) error {
		scanCalled++
		if scanCalled == 1 {
			return assert.AnError // simulate scan error on the first row
		}
		return scanSpanRowFn()(dest, src)
	}

	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectSpansQuery: {
				Rows: &clickhousetest.Rows[*dbmodel.SpanRow]{
					Data:   multipleSpans,
					ScanFn: scanFn,
				},
				Err: nil,
			},
		},
	}

	reader := NewReader(conn, testReaderConfig)
	iter := reader.FindTraces(context.Background(), tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
	})
	_, err := jiter.FlattenWithErrors(iter)
	require.ErrorContains(t, err, "failed to scan span row")
}

func TestFindTraces_ErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		driver      *clickhousetest.Driver
		expectedErr string
	}{
		{
			name: "QueryError",
			driver: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectSpansQuery: {
						Rows: nil,
						Err:  assert.AnError,
					},
				},
			},
			expectedErr: "failed to query traces",
		},
		{
			name: "ScanError",
			driver: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectSpansQuery: {
						Rows: &clickhousetest.Rows[*dbmodel.SpanRow]{
							Data:    singleSpan,
							ScanErr: assert.AnError,
						},
						Err: nil,
					},
				},
			},
			expectedErr: "failed to scan span row",
		},
		{
			name: "RowsError",
			driver: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectSpansQuery: {
						Rows: &clickhousetest.Rows[*dbmodel.SpanRow]{
							RowsErr: assert.AnError,
						},
					},
				},
			},
			expectedErr: "failed to read span rows",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := NewReader(test.driver, testReaderConfig)
			iter := reader.FindTraces(context.Background(), tracestore.TraceQueryParams{
				Attributes: pcommon.NewMap(),
			})
			_, err := jiter.FlattenWithErrors(iter)
			require.ErrorContains(t, err, test.expectedErr)
		})
	}
}

func TestFindTraces_BuildQueryError(t *testing.T) {
	orig := marshalValueForQuery
	t.Cleanup(func() { marshalValueForQuery = orig })

	marshalValueForQuery = func(pcommon.Value) (string, error) {
		return "", assert.AnError
	}

	attrs := pcommon.NewMap()
	attrs.PutEmptySlice("bad_slice").AppendEmpty()

	reader := NewReader(&clickhousetest.Driver{}, testReaderConfig)
	iter := reader.FindTraces(context.Background(), tracestore.TraceQueryParams{
		Attributes:  attrs,
		SearchDepth: 1,
	})
	_, err := jiter.FlattenWithErrors(iter)
	require.ErrorContains(t, err, "failed to build query")
}

func TestFindTraceIDs(t *testing.T) {
	driver := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectAttributeMetadata: {
				Rows: &clickhousetest.Rows[dbmodel.AttributeMetadata]{
					Data:   testAttributeMetadata,
					ScanFn: scanAttributeMetadataFn(),
				},
			},
			sql.SearchTraceIDsBase: {
				Rows: &clickhousetest.Rows[[]any]{
					Data:   testTraceIDsData,
					ScanFn: scanTraceIDFn(),
				},
				Err: nil,
			},
		},
	}
	reader := NewReader(driver, testReaderConfig)
	attributes := buildTestAttributes()

	iter := reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{
		ServiceName:   "serviceA",
		OperationName: "operationA",
		DurationMin:   1 * time.Nanosecond,
		DurationMax:   1 * time.Second,
		StartTimeMin:  now.Add(-1 * time.Hour),
		StartTimeMax:  now,
		Attributes:    attributes,
		SearchDepth:   5,
	})
	ids, err := jiter.FlattenWithErrors(iter)
	require.NoError(t, err)
	require.Len(t, driver.RecordedQueries, 2)
	verifyQuerySnapshot(t, driver.RecordedQueries...)
	require.Equal(t, []tracestore.FoundTraceID{
		{
			TraceID: pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}),
			Start:   now.Add(-1 * time.Hour),
			End:     now,
		},
		{
			TraceID: pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}),
		},
	}, ids)
}

func TestFindTraceIDs_SearchDepthExceedsMax(t *testing.T) {
	driver := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SearchTraceIDsBase: {
				Rows: &clickhousetest.Rows[[]any]{
					Data: [][]any{
						{
							"00000000000000000000000000000001",
							time.Now().Add(-1 * time.Hour),
							time.Now().Add(-1 * time.Minute),
						},
						{
							"00000000000000000000000000000002",
							time.Now().Add(-2 * time.Hour),
							time.Now().Add(-2 * time.Minute),
						},
					},
					ScanFn: scanTraceIDFn(),
				},
				Err: nil,
			},
		},
	}
	reader := NewReader(driver, testReaderConfig)
	iter := reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{
		SearchDepth: 10000,
	})
	_, err := jiter.FlattenWithErrors(iter)
	require.ErrorContains(t, err, "search depth 10000 exceeds maximum allowed 1000")
}

func TestFindTraceIDs_YieldFalseOnSuccessStopsIteration(t *testing.T) {
	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SearchTraceIDsBase: {
				Rows: &clickhousetest.Rows[[]any]{
					Data:   testTraceIDsData,
					ScanFn: scanTraceIDFn(),
				},
				Err: nil,
			},
		},
	}

	reader := NewReader(conn, testReaderConfig)
	findTraceIDsIter := reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
	})

	var gotTraceIDs []tracestore.FoundTraceID
	findTraceIDsIter(func(traceIDs []tracestore.FoundTraceID, err error) bool {
		require.NoError(t, err)
		gotTraceIDs = append(gotTraceIDs, traceIDs...)
		return false // stop iteration after the first trace ID
	})

	require.Len(t, gotTraceIDs, 1)
	require.Equal(t, []tracestore.FoundTraceID{
		{
			TraceID: pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}),
			Start:   now.Add(-1 * time.Hour),
			End:     now,
		},
	}, gotTraceIDs)
}

func TestFindTraceIDs_ScanErrorStopsIteration(t *testing.T) {
	scanCalled := 0

	scanFn := func(dest any, src []any) error {
		scanCalled++
		if scanCalled == 1 {
			return assert.AnError // simulate scan error on the first row
		}
		return scanTraceIDFn()(dest, src)
	}

	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SearchTraceIDsBase: {
				Rows: &clickhousetest.Rows[[]any]{
					Data:   testTraceIDsData,
					ScanFn: scanFn,
				},
				Err: nil,
			},
		},
	}

	reader := NewReader(conn, testReaderConfig)
	iter := reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
	})
	_, err := jiter.FlattenWithErrors(iter)
	require.ErrorContains(t, err, "failed to scan row")
}

func TestFindTraceIDs_DecodeErrorStopsIteration(t *testing.T) {
	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SearchTraceIDsBase: {
				Rows: &clickhousetest.Rows[[]any]{
					Data: [][]any{
						testTraceIDsData[0],
						{
							"0x",
							time.Now().Add(-2 * time.Hour),
							time.Now().Add(-2 * time.Minute),
						},
						{
							"invalid",
							time.Now().Add(-3 * time.Hour),
							time.Now().Add(-3 * time.Minute),
						},
						testTraceIDsData[1],
					},
					ScanFn: scanTraceIDFn(),
				},
				Err: nil,
			},
		},
	}

	reader := NewReader(conn, ReaderConfig{})
	iter := reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
	})
	_, err := jiter.FlattenWithErrors(iter)
	require.ErrorContains(t, err, "failed to decode trace ID")
}

func TestFindTraceIDs_DecodeErrorInvalidLengthStopsIteration(t *testing.T) {
	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SearchTraceIDsBase: {
				Rows: &clickhousetest.Rows[[]any]{
					Data: [][]any{
						{"12345678", now.Add(-2 * time.Hour), now.Add(-2 * time.Minute)},
					},
					ScanFn: scanTraceIDFn(),
				},
			},
		},
	}
	reader := NewReader(conn, testReaderConfig)
	iter := reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
	})
	_, err := jiter.FlattenWithErrors(iter)
	require.ErrorContains(t, err, "invalid trace ID length")
}

func TestFindTraceIDs_ErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		driver      *clickhousetest.Driver
		expectedErr string
	}{
		{
			name: "QueryError",
			driver: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SearchTraceIDsBase: {
						Rows: nil,
						Err:  assert.AnError,
					},
				},
			},
			expectedErr: "failed to query trace IDs",
		},
		{
			name: "ScanError",
			driver: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SearchTraceIDsBase: {
						Rows: &clickhousetest.Rows[[]any]{
							Data:    testTraceIDsData,
							ScanErr: assert.AnError,
						},
						Err: nil,
					},
				},
			},
			expectedErr: "failed to scan row",
		},
		{
			name: "DecodeError",
			driver: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SearchTraceIDsBase: {
						Rows: &clickhousetest.Rows[[]any]{
							Data: [][]any{
								{
									"0x",
									time.Now().Add(-1 * time.Hour),
									time.Now().Add(-1 * time.Minute),
								},
							},
							ScanFn: scanTraceIDFn(),
						},
						Err: nil,
					},
				},
			},
			expectedErr: "failed to decode trace ID",
		},
		{
			name: "RowsError",
			driver: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SearchTraceIDsBase: {
						Rows: &clickhousetest.Rows[[]any]{
							RowsErr: assert.AnError,
						},
					},
				},
			},
			expectedErr: "failed to read trace ID rows",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := NewReader(test.driver, ReaderConfig{})
			iter := reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{
				Attributes: pcommon.NewMap(),
			})
			_, err := jiter.FlattenWithErrors(iter)
			require.ErrorContains(t, err, test.expectedErr)
		})
	}
}

func TestFindTraceIDs_BuildQueryError(t *testing.T) {
	orig := marshalValueForQuery
	t.Cleanup(func() { marshalValueForQuery = orig })

	marshalValueForQuery = func(pcommon.Value) (string, error) {
		return "", assert.AnError
	}

	attrs := pcommon.NewMap()
	attrs.PutEmptyMap("bad_map").PutEmpty("key")

	reader := NewReader(&clickhousetest.Driver{}, testReaderConfig)
	iter := reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{
		Attributes:  attrs,
		SearchDepth: 1,
	})
	_, err := jiter.FlattenWithErrors(iter)
	require.ErrorContains(t, err, "failed to build query")
}

// scanTraceSummaryFn maps a pre-aggregated test row (as ClickHouse would return
// from the native summary query) into the positional destinations used by
// scanTraceSummaryRow.
func scanTraceSummaryFn() func(dest any, src []any) error {
	return func(dest any, src []any) error {
		ptrs, ok := dest.([]any)
		if !ok {
			return fmt.Errorf("expected []any for dest, got %T", dest)
		}
		if len(ptrs) != 11 {
			return fmt.Errorf("expected 11 destination arguments, got %d", len(ptrs))
		}
		if len(src) != len(ptrs) {
			return fmt.Errorf("expected %d source values, got %d", len(ptrs), len(src))
		}
		assign := func(i int, dst, val any) error {
			switch d := dst.(type) {
			case *string:
				v, ok := val.(string)
				if !ok {
					return fmt.Errorf("dest[%d]: expected string, got %T", i, val)
				}
				*d = v
			case *time.Time:
				v, ok := val.(time.Time)
				if !ok {
					return fmt.Errorf("dest[%d]: expected time.Time, got %T", i, val)
				}
				*d = v
			case *uint64:
				v, ok := val.(uint64)
				if !ok {
					return fmt.Errorf("dest[%d]: expected uint64, got %T", i, val)
				}
				*d = v
			case *[]string:
				v, ok := val.([]string)
				if !ok {
					return fmt.Errorf("dest[%d]: expected []string, got %T", i, val)
				}
				*d = v
			case *[]uint64:
				v, ok := val.([]uint64)
				if !ok {
					return fmt.Errorf("dest[%d]: expected []uint64, got %T", i, val)
				}
				*d = v
			default:
				return fmt.Errorf("dest[%d]: unsupported destination type %T", i, dst)
			}
			return nil
		}
		for i := range ptrs {
			if err := assign(i, ptrs[i], src[i]); err != nil {
				return err
			}
		}
		return nil
	}
}

func TestFindTraceSummaries_Success(t *testing.T) {
	summaryRow := []any{
		traceIDHex1,
		now.Add(-1 * time.Hour),
		now,
		uint64(3),
		uint64(1),
		"serviceA",
		"operationA",
		[]string{"serviceA", "serviceB"},
		[]uint64{2, 1},
		[]uint64{1, 0},
		uint64(0),
	}
	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			"argMinIf((s.service_name": {
				Rows: &clickhousetest.Rows[[]any]{
					Data:   [][]any{summaryRow},
					ScanFn: scanTraceSummaryFn(),
				},
			},
		},
	}

	reader := NewReader(conn, testReaderConfig)
	iter := reader.FindTraceSummaries(context.Background(), tracestore.TraceQueryParams{
		ServiceName:  "serviceA",
		Attributes:   pcommon.NewMap(),
		StartTimeMin: now.Add(-1 * time.Hour),
		StartTimeMax: now,
		SearchDepth:  10,
	})
	summaries, err := jiter.FlattenWithErrors(iter)
	require.NoError(t, err)
	require.Len(t, conn.RecordedQueries, 1)
	// The service filter from buildFindTraceIDsQuery is embedded in the summary query.
	require.Contains(t, conn.RecordedQueries[0], "s.service_name = ?")

	require.Len(t, summaries, 1)
	assert.Equal(t, pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}), summaries[0].TraceID)
	assert.Equal(t, "serviceA", summaries[0].RootServiceName)
	assert.Equal(t, "operationA", summaries[0].RootOperationName)
	assert.Equal(t, now.Add(-1*time.Hour), summaries[0].MinStartTime)
	assert.Equal(t, now, summaries[0].MaxEndTime)
	assert.Equal(t, 3, summaries[0].SpanCount)
	assert.Equal(t, 1, summaries[0].ErrorSpanCount)
	assert.Equal(t, 0, summaries[0].OrphanSpanCount)
	assert.Equal(t, []tracestore.ServiceSummary{
		{Name: "serviceA", SpanCount: 2, ErrorSpanCount: 1},
		{Name: "serviceB", SpanCount: 1, ErrorSpanCount: 0},
	}, summaries[0].Services)
}

func TestFindTraceSummaries_CrossService(t *testing.T) {
	summaryRow := []any{
		traceIDHex1,
		now.Add(-1 * time.Hour),
		now,
		uint64(1),
		uint64(0),
		"serviceA",
		"operationA",
		[]string{"serviceA"},
		[]uint64{1},
		[]uint64{0},
		uint64(0),
	}
	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			"argMinIf((s.service_name": {
				Rows: &clickhousetest.Rows[[]any]{
					Data:   [][]any{summaryRow},
					ScanFn: scanTraceSummaryFn(),
				},
			},
		},
	}

	reader := NewReader(conn, testReaderConfig)
	// No ServiceName: cross-service search must not inject a service filter.
	iter := reader.FindTraceSummaries(context.Background(), tracestore.TraceQueryParams{
		Attributes:  pcommon.NewMap(),
		SearchDepth: 10,
	})
	summaries, err := jiter.FlattenWithErrors(iter)
	require.NoError(t, err)
	require.Len(t, conn.RecordedQueries, 1)
	require.NotContains(t, conn.RecordedQueries[0], "s.service_name = ?")
	require.Len(t, summaries, 1)
	assert.Equal(t, 1, summaries[0].SpanCount)
}

func TestFindTraceSummaries_BuildQueryError(t *testing.T) {
	reader := NewReader(&clickhousetest.Driver{}, testReaderConfig)
	iter := reader.FindTraceSummaries(context.Background(), tracestore.TraceQueryParams{
		Attributes:  pcommon.NewMap(),
		SearchDepth: 10000, // exceeds testReaderConfig.MaxSearchDepth
	})
	_, err := jiter.FlattenWithErrors(iter)
	require.ErrorContains(t, err, "failed to build query")
}

func TestFindTraceSummaries_QueryError(t *testing.T) {
	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			"argMinIf((s.service_name": {
				Err: assert.AnError,
			},
		},
	}
	reader := NewReader(conn, testReaderConfig)
	iter := reader.FindTraceSummaries(context.Background(), tracestore.TraceQueryParams{
		Attributes:  pcommon.NewMap(),
		SearchDepth: 10,
	})
	_, err := jiter.FlattenWithErrors(iter)
	require.ErrorContains(t, err, "failed to query trace summaries")
}

func TestFindTraceSummaries_ScanError(t *testing.T) {
	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			"argMinIf((s.service_name": {
				Rows: &clickhousetest.Rows[[]any]{
					Data:    [][]any{{}}, // one row present so Next() returns true
					ScanErr: assert.AnError,
				},
			},
		},
	}
	reader := NewReader(conn, testReaderConfig)
	iter := reader.FindTraceSummaries(context.Background(), tracestore.TraceQueryParams{
		Attributes:  pcommon.NewMap(),
		SearchDepth: 10,
	})
	_, err := jiter.FlattenWithErrors(iter)
	require.ErrorContains(t, err, "failed to scan summary row")
}

func TestFindTraceSummaries_TraceIDDecodeError(t *testing.T) {
	badRow := []any{
		"not-valid-hex",
		now.Add(-1 * time.Hour),
		now,
		uint64(1),
		uint64(0),
		"serviceA",
		"operationA",
		[]string{"serviceA"},
		[]uint64{1},
		[]uint64{0},
		uint64(0),
	}
	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			"argMinIf((s.service_name": {
				Rows: &clickhousetest.Rows[[]any]{
					Data:   [][]any{badRow},
					ScanFn: scanTraceSummaryFn(),
				},
			},
		},
	}
	reader := NewReader(conn, testReaderConfig)
	iter := reader.FindTraceSummaries(context.Background(), tracestore.TraceQueryParams{
		Attributes:  pcommon.NewMap(),
		SearchDepth: 10,
	})
	_, err := jiter.FlattenWithErrors(iter)
	require.ErrorContains(t, err, "failed to decode trace ID")
}

func TestFindTraceSummaries_TraceIDInvalidLength(t *testing.T) {
	badRow := []any{
		"12345678", // valid hex, but 4 bytes instead of 16
		now.Add(-1 * time.Hour),
		now,
		uint64(1),
		uint64(0),
		"serviceA",
		"operationA",
		[]string{"serviceA"},
		[]uint64{1},
		[]uint64{0},
		uint64(0),
	}
	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			"argMinIf((s.service_name": {
				Rows: &clickhousetest.Rows[[]any]{
					Data:   [][]any{badRow},
					ScanFn: scanTraceSummaryFn(),
				},
			},
		},
	}
	reader := NewReader(conn, testReaderConfig)
	iter := reader.FindTraceSummaries(context.Background(), tracestore.TraceQueryParams{
		Attributes:  pcommon.NewMap(),
		SearchDepth: 10,
	})
	_, err := jiter.FlattenWithErrors(iter)
	require.ErrorContains(t, err, "invalid trace ID length")
}

func TestFindTraceSummaries_RowsError(t *testing.T) {
	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			"argMinIf((s.service_name": {
				Rows: &clickhousetest.Rows[[]any]{
					RowsErr: assert.AnError,
				},
			},
		},
	}
	reader := NewReader(conn, testReaderConfig)
	iter := reader.FindTraceSummaries(context.Background(), tracestore.TraceQueryParams{
		Attributes:  pcommon.NewMap(),
		SearchDepth: 10,
	})
	_, err := jiter.FlattenWithErrors(iter)
	require.ErrorContains(t, err, "failed to read summary rows")
}

func TestFindTraceSummaries_CloseError(t *testing.T) {
	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			"argMinIf((s.service_name": {
				Rows: &clickhousetest.Rows[[]any]{
					CloseErr: assert.AnError,
				},
			},
		},
	}
	reader := NewReader(conn, testReaderConfig)
	iter := reader.FindTraceSummaries(context.Background(), tracestore.TraceQueryParams{
		Attributes:  pcommon.NewMap(),
		SearchDepth: 10,
	})
	_, err := jiter.FlattenWithErrors(iter)
	require.ErrorContains(t, err, "failed to close rows")
}

func TestFindTraceSummaries_EarlyTermination(t *testing.T) {
	summaryRow := []any{
		traceIDHex1,
		now.Add(-1 * time.Hour),
		now,
		uint64(1),
		uint64(0),
		"serviceA",
		"operationA",
		[]string{"serviceA"},
		[]uint64{1},
		[]uint64{0},
		uint64(0),
	}
	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			"argMinIf((s.service_name": {
				Rows: &clickhousetest.Rows[[]any]{
					Data:   [][]any{summaryRow, summaryRow},
					ScanFn: scanTraceSummaryFn(),
				},
			},
		},
	}
	reader := NewReader(conn, testReaderConfig)
	iter := reader.FindTraceSummaries(context.Background(), tracestore.TraceQueryParams{
		Attributes:  pcommon.NewMap(),
		SearchDepth: 10,
	})
	count := 0
	iter(func(_ []tracestore.TraceSummary, err error) bool {
		require.NoError(t, err)
		count++
		return false // stop after the first batch
	})
	assert.Equal(t, 1, count)
}
