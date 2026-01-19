// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

var (
	testReaderConfig = ReaderConfig{
		DefaultSearchDepth: 100,
		MaxSearchDepth:     1000,
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
		{AttributeKey: "resource.checksum", Type: "bytes", Level: "resource"},
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
		name     string
		data     []*dbmodel.SpanRow
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
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SelectSpansByTraceID: {
						rows: &testRows[*dbmodel.SpanRow]{
							data:   tt.data,
							scanFn: scanSpanRowFn(),
						},
						err: nil,
					},
				},
			}

			reader := NewReader(conn, testReaderConfig)
			getTracesIter := reader.GetTraces(context.Background(), tracestore.GetTraceParams{
				TraceID: traceID,
			})
			traces, err := jiter.FlattenWithErrors(getTracesIter)

			require.NoError(t, err)
			require.Len(t, conn.recordedQueries, 1)
			verifyQuerySnapshot(t, conn.recordedQueries...)
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
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SelectSpansByTraceID: {
						rows: nil,
						err:  assert.AnError,
					},
				},
			},
			expectedErr: "failed to query trace",
		},
		{
			name: "ScanError",
			driver: &testDriver{
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SelectSpansByTraceID: {
						rows: &testRows[*dbmodel.SpanRow]{
							data:    singleSpan,
							scanErr: assert.AnError,
						},
						err: nil,
					},
				},
			},
			expectedErr: "failed to scan span row",
		},
		{
			name: "CloseError",
			driver: &testDriver{
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SelectSpansByTraceID: {
						rows: &testRows[*dbmodel.SpanRow]{
							data:     singleSpan,
							scanFn:   scanSpanRowFn(),
							closeErr: assert.AnError,
						},
						err: nil,
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

func TestGetTraces_ScanErrorContinues(t *testing.T) {
	scanCalled := 0

	scanFn := func(dest any, src *dbmodel.SpanRow) error {
		scanCalled++
		if scanCalled == 1 {
			return assert.AnError // simulate scan error on the first row
		}
		return scanSpanRowFn()(dest, src)
	}

	conn := &testDriver{
		t: t,
		queryResponses: map[string]*testQueryResponse{
			sql.SelectSpansByTraceID: {
				rows: &testRows[*dbmodel.SpanRow]{
					data:   multipleSpans,
					scanFn: scanFn,
				},
				err: nil,
			},
		},
	}

	reader := NewReader(conn, testReaderConfig)
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
		t: t,
		queryResponses: map[string]*testQueryResponse{
			sql.SelectSpansByTraceID: {
				rows: &testRows[*dbmodel.SpanRow]{
					data:   multipleSpans,
					scanFn: scanSpanRowFn(),
				},
				err: nil,
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
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SelectServices: {
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
						err: nil,
					},
				},
			},
			expected: []string{"serviceA", "serviceB", "serviceC"},
		},
		{
			name: "query error",
			conn: &testDriver{
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SelectServices: {
						rows: nil,
						err:  assert.AnError,
					},
				},
			},
			expectError: "failed to query services",
		},
		{
			name: "scan error",
			conn: &testDriver{
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SelectServices: {
						rows: &testRows[dbmodel.Service]{
							data: []dbmodel.Service{
								{Name: "serviceA"},
								{Name: "serviceB"},
								{Name: "serviceC"},
							},
							scanErr: assert.AnError,
						},
						err: nil,
					},
				},
			},
			expectError: "failed to scan row",
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
				require.Len(t, test.conn.recordedQueries, 1)
				verifyQuerySnapshot(t, test.conn.recordedQueries...)
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
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SelectOperationsAllKinds: {
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
						err: nil,
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
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SelectOperationsByKind: {
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
						err: nil,
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
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SelectOperationsAllKinds: {
						rows: nil,
						err:  assert.AnError,
					},
				},
			},
			expectError: "failed to query operations",
		},
		{
			name: "scan error",
			conn: &testDriver{
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SelectOperationsAllKinds: {
						rows: &testRows[dbmodel.Operation]{
							data: []dbmodel.Operation{
								{Name: "operationA"},
								{Name: "operationB"},
								{Name: "operationC"},
							},
							scanErr: assert.AnError,
						},
						err: nil,
					},
				},
			},
			expectError: "failed to scan row",
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
				require.Len(t, test.conn.recordedQueries, 1)
				verifyQuerySnapshot(t, test.conn.recordedQueries...)
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
			conn := &testDriver{
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SelectSpansQuery: {
						rows: &testRows[*dbmodel.SpanRow]{
							data:   tt.data,
							scanFn: scanSpanRowFn(),
						},
						err: nil,
					},
				},
			}

			reader := NewReader(conn, testReaderConfig)
			findTracesIter := reader.FindTraces(context.Background(), tracestore.TraceQueryParams{
				Attributes: pcommon.NewMap(),
			})
			traces, err := jiter.FlattenWithErrors(findTracesIter)

			require.NoError(t, err)
			require.Len(t, conn.recordedQueries, 1)
			verifyQuerySnapshot(t, conn.recordedQueries...)
			requireTracesEqual(t, tt.data, traces)
		})
	}
}

func TestFindTraces_WithFilters(t *testing.T) {
	conn := &testDriver{
		t: t,
		queryResponses: map[string]*testQueryResponse{
			sql.SelectAttributeMetadata: {
				rows: &testRows[dbmodel.AttributeMetadata]{
					data:   testAttributeMetadata,
					scanFn: scanAttributeMetadataFn(),
				},
			},
			sql.SelectSpansQuery: {
				rows: &testRows[*dbmodel.SpanRow]{
					data:   multipleSpans,
					scanFn: scanSpanRowFn(),
				},
				err: nil,
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
	require.Len(t, conn.recordedQueries, 2)
	verifyQuerySnapshot(t, conn.recordedQueries...)
	requireTracesEqual(t, multipleSpans, traces)
}

func TestFindTraces_SearchDepthExceedsMax(t *testing.T) {
	driver := &testDriver{
		t: t,
	}
	reader := NewReader(driver, testReaderConfig)
	iter := reader.FindTraces(context.Background(), tracestore.TraceQueryParams{
		SearchDepth: 10000,
		Attributes:  pcommon.NewMap(),
	})
	_, err := jiter.FlattenWithErrors(iter)
	require.ErrorContains(t, err, "search depth 10000 exceeds maximum allowed 1000")
}

func TestFindTraces_YieldFalseOnSuccessStopsIteration(t *testing.T) {
	conn := &testDriver{
		t: t,
		queryResponses: map[string]*testQueryResponse{
			sql.SelectSpansQuery: {
				rows: &testRows[*dbmodel.SpanRow]{
					data:   multipleSpans,
					scanFn: scanSpanRowFn(),
				},
				err: nil,
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

func TestFindTraces_ScanErrorContinues(t *testing.T) {
	scanCalled := 0

	scanFn := func(dest any, src *dbmodel.SpanRow) error {
		scanCalled++
		if scanCalled == 1 {
			return assert.AnError // simulate scan error on the first row
		}
		return scanSpanRowFn()(dest, src)
	}

	conn := &testDriver{
		t: t,
		queryResponses: map[string]*testQueryResponse{
			sql.SelectSpansQuery: {
				rows: &testRows[*dbmodel.SpanRow]{
					data:   multipleSpans,
					scanFn: scanFn,
				},
				err: nil,
			},
		},
	}

	reader := NewReader(conn, testReaderConfig)
	findTracesIter := reader.FindTraces(context.Background(), tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
	})

	expected := multipleSpans[1:] // skip the first span which caused the error
	for trace, err := range findTracesIter {
		if err != nil {
			require.ErrorIs(t, err, assert.AnError)
			continue
		}
		requireTracesEqual(t, expected, trace)
	}
}

func TestFindTraces_ErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		driver      *testDriver
		expectedErr string
	}{
		{
			name: "QueryError",
			driver: &testDriver{
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SelectSpansQuery: {
						rows: nil,
						err:  assert.AnError,
					},
				},
			},
			expectedErr: "failed to query traces",
		},
		{
			name: "ScanError",
			driver: &testDriver{
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SelectSpansQuery: {
						rows: &testRows[*dbmodel.SpanRow]{
							data:    singleSpan,
							scanErr: assert.AnError,
						},
						err: nil,
					},
				},
			},
			expectedErr: "failed to scan span row",
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

	reader := NewReader(&testDriver{t: t}, testReaderConfig)
	iter := reader.FindTraces(context.Background(), tracestore.TraceQueryParams{
		Attributes:  attrs,
		SearchDepth: 1,
	})
	_, err := jiter.FlattenWithErrors(iter)
	require.ErrorContains(t, err, "failed to build query")
}

func TestFindTraceIDs(t *testing.T) {
	driver := &testDriver{
		t: t,
		queryResponses: map[string]*testQueryResponse{
			sql.SelectAttributeMetadata: {
				rows: &testRows[dbmodel.AttributeMetadata]{
					data:   testAttributeMetadata,
					scanFn: scanAttributeMetadataFn(),
				},
			},
			sql.SearchTraceIDs: {
				rows: &testRows[[]any]{
					data:   testTraceIDsData,
					scanFn: scanTraceIDFn(),
				},
				err: nil,
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
	require.Len(t, driver.recordedQueries, 2)
	verifyQuerySnapshot(t, driver.recordedQueries...)
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
	driver := &testDriver{
		t: t,
		queryResponses: map[string]*testQueryResponse{
			sql.SearchTraceIDs: {
				rows: &testRows[[]any]{
					data: [][]any{
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
					scanFn: scanTraceIDFn(),
				},
				err: nil,
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
	conn := &testDriver{
		t: t,
		queryResponses: map[string]*testQueryResponse{
			sql.SearchTraceIDs: {
				rows: &testRows[[]any]{
					data:   testTraceIDsData,
					scanFn: scanTraceIDFn(),
				},
				err: nil,
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

func TestFindTraceIDs_ScanErrorContinues(t *testing.T) {
	scanCalled := 0

	scanFn := func(dest any, src []any) error {
		scanCalled++
		if scanCalled == 1 {
			return assert.AnError // simulate scan error on the first row
		}
		return scanTraceIDFn()(dest, src)
	}

	conn := &testDriver{
		t: t,
		queryResponses: map[string]*testQueryResponse{
			sql.SearchTraceIDs: {
				rows: &testRows[[]any]{
					data:   testTraceIDsData,
					scanFn: scanFn,
				},
				err: nil,
			},
		},
	}

	reader := NewReader(conn, testReaderConfig)
	findTraceIDsIter := reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
	})

	expected := []tracestore.FoundTraceID{
		{
			TraceID: pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}),
		},
	}

	for traceID, err := range findTraceIDsIter {
		if err != nil {
			require.ErrorIs(t, err, assert.AnError)
			continue
		}
		require.Equal(t, expected, traceID)
	}
}

func TestFindTraceIDs_DecodeErrorContinues(t *testing.T) {
	conn := &testDriver{
		t: t,
		queryResponses: map[string]*testQueryResponse{
			sql.SearchTraceIDs: {
				rows: &testRows[[]any]{
					data: [][]any{
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
					scanFn: scanTraceIDFn(),
				},
				err: nil,
			},
		},
	}

	reader := NewReader(conn, ReaderConfig{})
	findTraceIDsIter := reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
	})

	expectedValidTraceIDs := []tracestore.FoundTraceID{
		{
			TraceID: pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}),
			Start:   now.Add(-1 * time.Hour),
			End:     now,
		},
		{
			TraceID: pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}),
		},
	}

	var gotTraceIDs []tracestore.FoundTraceID
	var errorCount int

	for traceID, err := range findTraceIDsIter {
		if err != nil {
			require.ErrorContains(t, err, "failed to decode trace ID")
			errorCount++
			continue
		}
		gotTraceIDs = append(gotTraceIDs, traceID...)
	}

	require.Equal(t, 2, errorCount)
	require.Equal(t, expectedValidTraceIDs, gotTraceIDs)
}

func TestFindTraceIDs_ErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		driver      *testDriver
		expectedErr string
	}{
		{
			name: "QueryError",
			driver: &testDriver{
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SearchTraceIDs: {
						rows: nil,
						err:  assert.AnError,
					},
				},
			},
			expectedErr: "failed to query trace IDs",
		},
		{
			name: "ScanError",
			driver: &testDriver{
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SearchTraceIDs: {
						rows: &testRows[[]any]{
							data:    testTraceIDsData,
							scanErr: assert.AnError,
						},
						err: nil,
					},
				},
			},
			expectedErr: "failed to scan row",
		},
		{
			name: "DecodeError",
			driver: &testDriver{
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SearchTraceIDs: {
						rows: &testRows[[]any]{
							data: [][]any{
								{
									"0x",
									time.Now().Add(-1 * time.Hour),
									time.Now().Add(-1 * time.Minute),
								},
							},
							scanFn: scanTraceIDFn(),
						},
						err: nil,
					},
				},
			},
			expectedErr: "failed to decode trace ID",
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

	reader := NewReader(&testDriver{t: t}, testReaderConfig)
	iter := reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{
		Attributes:  attrs,
		SearchDepth: 1,
	})
	_, err := jiter.FlattenWithErrors(iter)
	require.ErrorContains(t, err, "failed to build query")
}

func TestBuildFindTraceIDsQuery_MarshalErrors(t *testing.T) {
	orig := marshalValueForQuery
	t.Cleanup(func() { marshalValueForQuery = orig })
	marshalValueForQuery = func(pcommon.Value) (string, error) {
		return "", assert.AnError
	}

	t.Run("marshal slice error", func(t *testing.T) {
		attrs := pcommon.NewMap()
		s := attrs.PutEmptySlice("bad_slice")
		s.AppendEmpty()

		reader := NewReader(&testDriver{t: t}, testReaderConfig)
		_, _, err := reader.buildFindTraceIDsQuery(t.Context(), tracestore.TraceQueryParams{Attributes: attrs})

		require.Error(t, err)
		require.ErrorContains(t, err, "failed to marshal slice attribute")
	})

	t.Run("marshal map error", func(t *testing.T) {
		attrs := pcommon.NewMap()
		m := attrs.PutEmptyMap("bad_map")
		m.PutEmpty("key")

		reader := NewReader(&testDriver{t: t}, testReaderConfig)
		_, _, err := reader.buildFindTraceIDsQuery(t.Context(), tracestore.TraceQueryParams{Attributes: attrs})

		require.Error(t, err)
		require.ErrorContains(t, err, "failed to marshal map attribute")
	})
}

func TestBuildFindTraceIDsQuery_AttributeMetadataError(t *testing.T) {
	td := &testDriver{
		t: t,
		queryResponses: map[string]*testQueryResponse{
			sql.SelectAttributeMetadata: {
				rows: nil,
				err:  assert.AnError,
			},
		},
	}

	reader := NewReader(td, testReaderConfig)
	_, _, err := reader.buildFindTraceIDsQuery(t.Context(), tracestore.TraceQueryParams{Attributes: buildTestAttributes()})
	require.ErrorContains(t, err, "failed to get attribute metadata")
}

func TestBuildStringAttributeCondition_Errors(t *testing.T) {
	cases := []struct {
		name        string
		attrValue   string
		metadata    attributeMetadata
		expectedErr string
	}{
		{
			name:      "parse bool fails",
			attrValue: "not-bool",
			metadata: attributeMetadata{
				"k": {"span": {"bool"}},
			},
			expectedErr: "failed to parse bool attribute",
		},
		{
			name:      "parse double fails",
			attrValue: "not-float",
			metadata: attributeMetadata{
				"k": {"span": {"double"}},
			},
			expectedErr: "failed to parse double attribute",
		},
		{
			name:      "parse int fails",
			attrValue: "not-int",
			metadata: attributeMetadata{
				"k": {"span": {"int"}},
			},
			expectedErr: "failed to parse int attribute",
		},
		{
			name:      "decode bytes fails",
			attrValue: "!not-base64!",
			metadata: attributeMetadata{
				"k": {"span": {"bytes"}},
			},
			expectedErr: "failed to decode bytes attribute",
		},
		{
			name:      "unsupported type",
			attrValue: "whatever",
			metadata: attributeMetadata{
				"k": {"span": {"unknown"}},
			},
			expectedErr: "unsupported attribute type",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			attr := pcommon.NewValueStr(tc.attrValue)
			var q strings.Builder
			var args []any

			err := buildStringAttributeCondition(&q, &args, "k", attr, tc.metadata)
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.expectedErr)
		})
	}
}
