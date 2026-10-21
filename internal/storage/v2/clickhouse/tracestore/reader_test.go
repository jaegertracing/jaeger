// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"fmt"
	stditer "iter"
	"reflect"
	"testing"
	"time"

	chdriver "github.com/ClickHouse/clickhouse-go/v2/lib/driver"
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

func flattenPageChunks[T any](seq stditer.Seq2[tracestore.PageChunk[[]T], error]) ([]T, error) {
	var results []T
	for chunk, err := range seq {
		if err != nil {
			return nil, err
		}
		if chunk.NextPageToken != "" {
			return nil, errors.New("unexpected next page token")
		}
		results = append(results, chunk.Results...)
	}
	return results, nil
}

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
		if len(ptrs) == 1 {
			*ptrs[0].(*string) = src[0].(string)
			return nil
		}
		if len(ptrs) != 3 {
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
	rows := &clickhousetest.Rows[*dbmodel.SpanRow]{
		Data:   multipleSpans,
		ScanFn: scanSpanRowFn(),
	}
	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectSpansByTraceID: {Rows: rows},
		},
	}

	reader := NewReader(conn, testReaderConfig)
	getTracesIter := reader.GetTraces(context.Background(), tracestore.GetTraceParams{
		TraceID: traceID,
	}, tracestore.GetTraceParams{
		TraceID: pcommon.TraceID{15: 2},
	})

	var gotTraces []ptrace.Traces
	getTracesIter(func(traces []ptrace.Traces, err error) bool {
		require.NoError(t, err)
		gotTraces = append(gotTraces, traces...)
		return false // stop iteration after the first span
	})

	require.Len(t, gotTraces, 1)
	requireTracesEqual(t, multipleSpans[0:1], gotTraces)
	// The consumer stopped during the first trace, so the second trace is never queried.
	require.Len(t, conn.RecordedQueries, 1)
	require.True(t, rows.Closed)
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

			addTraceSearchResponses(conn)
			reader := NewReader(conn, testReaderConfig)
			findTracesIter := reader.FindTraces(context.Background(), tracestore.TraceQueryParams{
				Attributes: pcommon.NewMap(),
			})
			traces, err := jiter.FlattenWithErrors(findTracesIter)

			require.NoError(t, err)
			require.Len(t, conn.RecordedQueries, 3)
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

	addTraceSearchResponses(conn)
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
	require.Len(t, conn.RecordedQueries, 4)
	verifyQuerySnapshot(t, conn.RecordedQueries...)
	requireTracesEqual(t, multipleSpans, traces)
}

func TestFindTraces_SearchDepthExceedsMax(t *testing.T) {
	driver := &clickhousetest.Driver{}
	addTraceSearchResponses(driver)
	reader := NewReader(driver, testReaderConfig)
	iter := reader.FindTraces(context.Background(), tracestore.TraceQueryParams{
		SearchDepth: 10000,
		Attributes:  pcommon.NewMap(),
	})
	_, err := jiter.FlattenWithErrors(iter)
	require.ErrorContains(t, err, "search depth 10000 exceeds maximum allowed 1000")
}

func TestBuildFindTraceIDsQuery_DefaultSearchDepthExceedsMax(t *testing.T) {
	config := testReaderConfig
	config.DefaultSearchDepth = config.MaxSearchDepth + 1
	reader := NewReader(&clickhousetest.Driver{}, config)
	_, _, err := reader.buildFindTraceIDsQuery(context.Background(), tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
	})
	require.EqualError(t, err, "search depth 1001 exceeds maximum allowed 1000")
}

func TestFindTraces_YieldFalseOnSuccessStopsIteration(t *testing.T) {
	rows := &clickhousetest.Rows[*dbmodel.SpanRow]{
		Data:   multipleSpans,
		ScanFn: scanSpanRowFn(),
	}
	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectSpansQuery: {Rows: rows},
		},
	}

	addTraceSearchResponses(conn)
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
	require.True(t, rows.Closed)
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

	addTraceSearchResponses(conn)
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

	addTraceSearchResponses(conn)
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
			addTraceSearchResponses(test.driver)
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
	addTraceSearchResponses(driver)
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
	ids, err := flattenPageChunks(iter)
	require.NoError(t, err)
	require.Len(t, driver.RecordedQueries, 3)
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
	addTraceSearchResponses(driver)
	reader := NewReader(driver, testReaderConfig)
	iter := reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{
		SearchDepth: 10000,
	})
	_, err := flattenPageChunks(iter)
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

	addTraceSearchResponses(conn)
	reader := NewReader(conn, testReaderConfig)
	findTraceIDsIter := reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
	})

	var gotTraceIDs []tracestore.FoundTraceID
	findTraceIDsIter(func(chunk tracestore.PageChunk[[]tracestore.FoundTraceID], err error) bool {
		require.NoError(t, err)
		gotTraceIDs = append(gotTraceIDs, chunk.Results...)
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

	addTraceSearchResponses(conn)
	reader := NewReader(conn, testReaderConfig)
	iter := reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
	})
	_, err := flattenPageChunks(iter)
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

	addTraceSearchResponses(conn)
	reader := NewReader(conn, ReaderConfig{})
	iter := reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
	})
	_, err := flattenPageChunks(iter)
	require.ErrorContains(t, err, "failed to decode trace ID")
}

func TestFindTraceIDs_ShortTraceIDIsAnError(t *testing.T) {
	// A trace_id column that is valid hex but shorter than 16 bytes must be
	// reported as a decode error rather than panic in the array conversion.
	conn := &clickhousetest.Driver{
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SearchTraceIDsBase: {
				Rows: &clickhousetest.Rows[[]any]{
					Data: [][]any{
						{
							"0001",
							time.Now().Add(-2 * time.Hour),
							time.Now().Add(-2 * time.Minute),
						},
					},
					ScanFn: scanTraceIDFn(),
				},
			},
		},
	}

	addTraceSearchResponses(conn)
	reader := NewReader(conn, ReaderConfig{})
	iter := reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
	})
	_, err := flattenPageChunks(iter)
	require.ErrorContains(t, err, "failed to decode trace ID")
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
		{
			name: "CloseError",
			driver: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SearchTraceIDsBase: {
						Rows: &clickhousetest.Rows[[]any]{
							CloseErr: assert.AnError,
						},
					},
				},
			},
			expectedErr: "failed to close rows",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			addTraceSearchResponses(test.driver)
			reader := NewReader(test.driver, ReaderConfig{})
			iter := reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{
				Attributes: pcommon.NewMap(),
			})
			_, err := flattenPageChunks(iter)
			require.ErrorContains(t, err, test.expectedErr)
			// A failed candidate search stops the pipeline before the bounds lookup.
			require.Len(t, test.driver.RecordedQueries, 1)
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
	_, err := flattenPageChunks(iter)
	require.ErrorContains(t, err, "failed to build query")
}

// The search SQL appends every predicate conditionally, so ClickHouse answers a query
// that omits the service name (RFC 0013).
func TestReader_SearchCapabilities(t *testing.T) {
	caps, err := (&Reader{}).SearchCapabilities(context.Background())
	require.NoError(t, err)
	filter := FilterCapabilities()
	assert.Equal(t, tracestore.SearchCapabilities{
		WithoutServiceName:  true,
		SameSpanConjunction: true,
		Filter:              &filter,
	}, caps)
}

func addTraceSearchResponses(conn *clickhousetest.Driver) {
	if conn.QueryResponses == nil {
		conn.QueryResponses = make(map[string]*clickhousetest.QueryResponse)
	}
	if conn.QueryResponses[sql.SearchTraceIDsBase] == nil {
		conn.QueryResponses[sql.SearchTraceIDsBase] = &clickhousetest.QueryResponse{
			Rows: &clickhousetest.Rows[[]any]{Data: testTraceIDsData[:1], ScanFn: scanTraceIDFn()},
		}
	}
	conn.QueryResponses[sql.SelectTraceIDTimestamps] = &clickhousetest.QueryResponse{
		Rows: &clickhousetest.Rows[[]any]{Data: testTraceIDsData, ScanFn: scanTraceIDFn()},
	}
}

// TestFindTraces_StagedQueries checks the arguments of the three search statements: the
// candidate search, the bounds lookup over exactly the candidate IDs, and the span fetch with
// each trace's own bounds in candidate order. The statement text is covered by the snapshots.
func TestFindTraces_StagedQueries(t *testing.T) {
	start := time.Unix(1700000000, 123456789)
	end := start.Add(3*time.Second + 987654321*time.Nanosecond)
	candidates := &clickhousetest.Rows[[]any]{Data: [][]any{{traceIDHex1}, {traceIDHex2}}, ScanFn: scanTraceIDFn()}
	timestamps := &clickhousetest.Rows[[]any]{
		// Bounds rows need not arrive in candidate order.
		Data:   [][]any{{traceIDHex2, start.Add(time.Second), end}, {traceIDHex1, start, end.Add(-time.Second)}},
		ScanFn: scanTraceIDFn(),
	}
	spans := &clickhousetest.Rows[*dbmodel.SpanRow]{Data: multipleSpans, ScanFn: scanSpanRowFn()}
	conn := &clickhousetest.Driver{QueryResponses: map[string]*clickhousetest.QueryResponse{
		sql.SearchTraceIDsBase:      {Rows: candidates},
		sql.SelectTraceIDTimestamps: {Rows: timestamps},
		sql.SelectSpansQuery:        {Rows: spans},
	}}
	query := tracestore.TraceQueryParams{
		ServiceName:  "serviceA",
		Attributes:   pcommon.NewMap(),
		SearchDepth:  2,
		StartTimeMin: start.Add(time.Second),
		StartTimeMax: end.Add(-time.Second),
	}
	reader := NewReader(conn, testReaderConfig)
	traces, err := jiter.FlattenWithErrors(reader.FindTraces(t.Context(), query))
	require.NoError(t, err)
	requireTracesEqual(t, multipleSpans, traces)
	require.Equal(t, [][]any{
		{"serviceA", query.StartTimeMin, query.StartTimeMax, uint32(2)},
		{[]string{traceIDHex1, traceIDHex2}},
		{
			traceIDHex1, start.UnixNano(), end.Add(-time.Second).UnixNano(),
			traceIDHex2, start.Add(time.Second).UnixNano(), end.UnixNano(),
		},
	}, conn.RecordedArgs)
	require.True(t, candidates.Closed)
	require.True(t, timestamps.Closed)
	require.True(t, spans.Closed)
}

func TestFindTraces_NoCandidates(t *testing.T) {
	for _, method := range []string{"FindTraces", "FindTraceIDs"} {
		t.Run(method, func(t *testing.T) {
			rows := &clickhousetest.Rows[[]any]{}
			conn := &clickhousetest.Driver{QueryResponses: map[string]*clickhousetest.QueryResponse{
				sql.SearchTraceIDsBase: {Rows: rows},
			}}
			reader := NewReader(conn, testReaderConfig)
			query := tracestore.TraceQueryParams{Attributes: pcommon.NewMap()}
			if method == "FindTraces" {
				traces, err := jiter.FlattenWithErrors(reader.FindTraces(t.Context(), query))
				require.NoError(t, err)
				require.Empty(t, traces)
			} else {
				ids, err := flattenPageChunks(reader.FindTraceIDs(t.Context(), query))
				require.NoError(t, err)
				require.Empty(t, ids)
			}
			require.Len(t, conn.RecordedQueries, 1)
			require.True(t, rows.Closed)
		})
	}
}

// TestFindTraceIDs_MissingTimestamps checks that a candidate without a bounds row keeps zero
// bounds and that a bounds row for an unknown ID is ignored.
func TestFindTraceIDs_MissingTimestamps(t *testing.T) {
	candidates := &clickhousetest.Rows[[]any]{Data: [][]any{{traceIDHex1}, {traceIDHex2}}, ScanFn: scanTraceIDFn()}
	timestamps := &clickhousetest.Rows[[]any]{
		Data:   [][]any{{traceIDHex2, now.Add(-time.Hour), now}, {"00000000000000000000000000000003", now, now}},
		ScanFn: scanTraceIDFn(),
	}
	conn := &clickhousetest.Driver{QueryResponses: map[string]*clickhousetest.QueryResponse{
		sql.SearchTraceIDsBase: {Rows: candidates}, sql.SelectTraceIDTimestamps: {Rows: timestamps},
	}}
	reader := NewReader(conn, testReaderConfig)
	ids, err := flattenPageChunks(reader.FindTraceIDs(t.Context(), tracestore.TraceQueryParams{Attributes: pcommon.NewMap()}))
	require.NoError(t, err)
	require.Equal(t, []tracestore.FoundTraceID{
		{TraceID: pcommon.TraceID{15: 1}},
		{TraceID: pcommon.TraceID{15: 2}, Start: now.Add(-time.Hour), End: now},
	}, ids)
}

func TestFindTraceIDs_TimestampStageErrors(t *testing.T) {
	for _, tc := range []struct {
		name     string
		rows     *clickhousetest.Rows[[]any]
		queryErr error
		expected string
	}{
		{name: "query", queryErr: assert.AnError, expected: "failed to query trace timestamps"},
		{name: "canceled", queryErr: context.Canceled, expected: "failed to query trace timestamps"},
		{name: "scan", rows: &clickhousetest.Rows[[]any]{Data: testTraceIDsData, ScanErr: assert.AnError}, expected: "failed to scan row"},
		{name: "decode", rows: &clickhousetest.Rows[[]any]{Data: [][]any{{"invalid", now, now}}, ScanFn: scanTraceIDFn()}, expected: "failed to decode trace ID"},
		{name: "iteration", rows: &clickhousetest.Rows[[]any]{RowsErr: assert.AnError}, expected: "failed to read trace timestamp rows"},
		{name: "close", rows: &clickhousetest.Rows[[]any]{CloseErr: assert.AnError}, expected: "failed to close rows"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidates := &clickhousetest.Rows[[]any]{Data: testTraceIDsData, ScanFn: scanTraceIDFn()}
			conn := &clickhousetest.Driver{QueryResponses: map[string]*clickhousetest.QueryResponse{
				sql.SearchTraceIDsBase: {Rows: candidates},
			}}
			if tc.rows != nil {
				conn.QueryResponses[sql.SelectTraceIDTimestamps] = &clickhousetest.QueryResponse{Rows: tc.rows}
			} else {
				conn.QueryResponses[sql.SelectTraceIDTimestamps] = &clickhousetest.QueryResponse{Err: tc.queryErr}
			}
			reader := NewReader(conn, testReaderConfig)
			_, err := flattenPageChunks(reader.FindTraceIDs(t.Context(), tracestore.TraceQueryParams{Attributes: pcommon.NewMap()}))
			require.ErrorContains(t, err, tc.expected)
			if tc.queryErr != nil {
				require.ErrorIs(t, err, tc.queryErr)
			}
			require.Len(t, conn.RecordedQueries, 2)
			require.True(t, candidates.Closed)
			if tc.rows != nil {
				require.True(t, tc.rows.Closed)
			}
		})
	}
}

func TestFindTraces_CloseError(t *testing.T) {
	conn := &clickhousetest.Driver{QueryResponses: map[string]*clickhousetest.QueryResponse{
		sql.SelectSpansQuery: {Rows: &clickhousetest.Rows[*dbmodel.SpanRow]{CloseErr: assert.AnError}},
	}}
	addTraceSearchResponses(conn)
	reader := NewReader(conn, testReaderConfig)
	_, err := jiter.FlattenWithErrors(reader.FindTraces(t.Context(), tracestore.TraceQueryParams{Attributes: pcommon.NewMap()}))
	require.ErrorContains(t, err, "failed to close rows")
	require.ErrorIs(t, err, assert.AnError)
}

func TestReadRows(t *testing.T) {
	scanErr := errors.New("scan failed")
	for _, tc := range []struct {
		name     string
		rows     *clickhousetest.Rows[string]
		scan     func(chdriver.Rows) error
		scanned  int
		expected []error
	}{
		{
			name:    "every row scanned",
			rows:    &clickhousetest.Rows[string]{Data: []string{"a", "b"}},
			scanned: 2,
		},
		{
			name:     "scan error stops the loop and joins the close error",
			rows:     &clickhousetest.Rows[string]{Data: []string{"a", "b"}, CloseErr: assert.AnError},
			scan:     func(chdriver.Rows) error { return scanErr },
			scanned:  1,
			expected: []error{scanErr, assert.AnError},
		},
		{
			name:     "iteration error",
			rows:     &clickhousetest.Rows[string]{RowsErr: assert.AnError},
			expected: []error{assert.AnError},
		},
		{
			name:     "stop reading closes the rows and returns only the stop signal",
			rows:     &clickhousetest.Rows[string]{Data: []string{"a", "b"}, RowsErr: assert.AnError, CloseErr: assert.AnError},
			scan:     func(chdriver.Rows) error { return errStopReading },
			scanned:  1,
			expected: []error{errStopReading},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scanned := 0
			err := readRows(tc.rows, "test", func(rows chdriver.Rows) error {
				scanned++
				if tc.scan != nil {
					return tc.scan(rows)
				}
				return nil
			})
			require.Equal(t, tc.scanned, scanned)
			require.True(t, tc.rows.Closed)
			if len(tc.expected) == 0 {
				require.NoError(t, err)
				return
			}
			for _, expected := range tc.expected {
				require.ErrorIs(t, err, expected)
			}
			if errors.Is(err, errStopReading) {
				require.Equal(t, errStopReading, err, "the stop signal carries no other error")
			}
			if tc.rows.RowsErr != nil && tc.scan == nil {
				require.ErrorContains(t, err, "failed to read test rows")
			}
		})
	}
}
