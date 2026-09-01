// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	expression "github.com/jaegertracing/jaeger-idl/query/expression/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/mocks"
)

// TestTraceReader_SearchCapabilities pins the declaration the query service reads before it
// dispatches: the search query adds its process.serviceName clause only when the query
// carries a service name, so Elasticsearch/OpenSearch answers a query that omits it (RFC
// 0013), and it declares the part of the structured filter model it lowers (RFC 0005 §7).
func TestTraceReader_SearchCapabilities(t *testing.T) {
	caps, err := (&TraceReader{}).SearchCapabilities(context.Background())
	require.NoError(t, err)
	filter := core.FilterCapabilities()
	assert.Equal(t, tracestore.SearchCapabilities{
		WithoutServiceName:  true,
		SameSpanConjunction: true,
		Filter:              &filter,
	}, caps)
}

func TestTraceReader_GetServices(t *testing.T) {
	coreReader := &mocks.Reader{}
	reader := TraceReader{spanReader: coreReader}
	services := []string{"service1", "service2"}
	coreReader.On("GetServices", mock.Anything).Return(services, nil)
	actual, err := reader.GetServices(context.Background())
	require.NoError(t, err)
	require.Equal(t, services, actual)
}

func TestTraceReader_GetOperations(t *testing.T) {
	coreReader := &mocks.Reader{}
	reader := TraceReader{spanReader: coreReader}
	operations := []dbmodel.Operation{
		{
			Name:     "op-1",
			SpanKind: "kind--1",
		},
		{
			Name:     "op-2",
			SpanKind: "kind--2",
		},
	}
	coreReader.On("GetOperations", mock.Anything, mock.Anything).Return(operations, nil)
	expected := []tracestore.Operation{
		{
			Name:     "op-1",
			SpanKind: "kind--1",
		},
		{
			Name:     "op-2",
			SpanKind: "kind--2",
		},
	}
	actual, err := reader.GetOperations(context.Background(), tracestore.OperationQueryParams{})
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func TestTraceReader_GetOperations_Error(t *testing.T) {
	coreReader := &mocks.Reader{}
	reader := TraceReader{spanReader: coreReader}
	coreReader.On("GetOperations", mock.Anything, mock.Anything).Return(nil, errors.New("error"))
	operations, err := reader.GetOperations(context.Background(), tracestore.OperationQueryParams{})
	require.EqualError(t, err, "error")
	require.Nil(t, operations)
}

func TestTraceReader_GetTraces(t *testing.T) {
	coreReader := &mocks.Reader{}
	reader := TraceReader{spanReader: coreReader}
	tracesStr, spanStr := loadFixtures(t, 1)
	var span dbmodel.Span
	require.NoError(t, json.Unmarshal(spanStr, &span))
	dbTrace := dbmodel.Trace{Spans: []dbmodel.Span{span}}
	span.TraceID = "00000000000000020000000000000000"
	dbTrace2 := dbmodel.Trace{Spans: []dbmodel.Span{span}}
	coreReader.On("GetTraces", mock.Anything, mock.Anything).Return([]dbmodel.Trace{dbTrace, dbTrace2}, nil)
	traces := reader.GetTraces(context.Background(), tracestore.GetTraceParams{})
	for td, err := range traces {
		require.NoError(t, err)
		assert.Len(t, td, 1)
		testTraces(t, tracesStr, td[0])
		break
	}
}

func testTraceReaderGetTracesAndFindTracesErrors(t *testing.T, fxnName string, actualTraces func(r TraceReader) iter.Seq2[[]ptrace.Traces, error]) {
	tests := []struct {
		name        string
		expectedErr string
		mockFxn     func(m *mocks.Reader)
	}{
		{
			name:        "some error from core reader",
			expectedErr: "some error",
			mockFxn: func(m *mocks.Reader) {
				m.On(fxnName, mock.Anything, mock.Anything).Return(nil, errors.New("some error"))
			},
		},
		{
			name: "conversion error",
			mockFxn: func(m *mocks.Reader) {
				dbTraces := []dbmodel.Trace{
					{
						Spans: []dbmodel.Span{
							{
								TraceID: "wrong-trace-id",
							},
						},
					},
				}
				m.On(fxnName, mock.Anything, mock.Anything).Return(dbTraces, nil)
			},
			expectedErr: "encoding/hex: invalid byte: U+0077 'w'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coreReader := &mocks.Reader{}
			reader := TraceReader{spanReader: coreReader}
			tt.mockFxn(coreReader)
			traces := actualTraces(reader)
			for trace, err := range traces {
				require.Nil(t, trace)
				require.ErrorContains(t, err, tt.expectedErr)
			}
		})
	}
}

func TestTraceReader_GetTraces_Errors(t *testing.T) {
	testTraceReaderGetTracesAndFindTracesErrors(t, "GetTraces", func(r TraceReader) iter.Seq2[[]ptrace.Traces, error] {
		return r.GetTraces(context.Background(), tracestore.GetTraceParams{})
	})
}

// testSkipsConversionErrorAndContinues verifies that one trace's conversion
// failure is surfaced as a per-trace error without abandoning the rest of the
// batch (issue #8899). The batch order is good, bad, good: both good traces must
// still be yielded, and the bad one reported as a single error.
func testSkipsConversionErrorAndContinues(t *testing.T, fxnName string, actualTraces func(r TraceReader) iter.Seq2[[]ptrace.Traces, error]) {
	coreReader := &mocks.Reader{}
	reader := TraceReader{spanReader: coreReader}

	_, spanStr := loadFixtures(t, 1)
	var good dbmodel.Span
	require.NoError(t, json.Unmarshal(spanStr, &good))
	goodTrace1 := dbmodel.Trace{Spans: []dbmodel.Span{good}}
	good2 := good
	good2.TraceID = "00000000000000020000000000000000"
	goodTrace2 := dbmodel.Trace{Spans: []dbmodel.Span{good2}}
	badTrace := dbmodel.Trace{Spans: []dbmodel.Span{{TraceID: "wrong-trace-id"}}}
	coreReader.On(fxnName, mock.Anything, mock.Anything).
		Return([]dbmodel.Trace{goodTrace1, badTrace, goodTrace2}, nil)

	var goodCount, errCount int
	for td, err := range actualTraces(reader) {
		if err != nil {
			errCount++
			require.ErrorContains(t, err, "encoding/hex: invalid byte")
			require.Nil(t, td)
			continue
		}
		goodCount++
		require.Len(t, td, 1)
	}
	assert.Equal(t, 2, goodCount, "both good traces are yielded despite the bad one in the middle")
	assert.Equal(t, 1, errCount, "the bad trace is surfaced as one per-trace error, not an abort")
}

func TestTraceReader_GetTraces_SkipsConversionErrorAndContinues(t *testing.T) {
	testSkipsConversionErrorAndContinues(t, "GetTraces", func(r TraceReader) iter.Seq2[[]ptrace.Traces, error] {
		return r.GetTraces(context.Background(), tracestore.GetTraceParams{})
	})
}

func TestTraceReader_FindTraces_SkipsConversionErrorAndContinues(t *testing.T) {
	testSkipsConversionErrorAndContinues(t, "FindTraces", func(r TraceReader) iter.Seq2[[]ptrace.Traces, error] {
		return r.FindTraces(context.Background(), tracestore.TraceQueryParams{
			Attributes: pcommon.NewMap(),
		})
	})
}

// testStopsWhenConsumerBreaksOnError verifies that a consumer breaking out of
// the iteration while handling a per-trace conversion error stops it cleanly
// instead of continuing to the rest of the batch (issue #8899).
func testStopsWhenConsumerBreaksOnError(t *testing.T, fxnName string, actualTraces func(r TraceReader) iter.Seq2[[]ptrace.Traces, error]) {
	coreReader := &mocks.Reader{}
	reader := TraceReader{spanReader: coreReader}

	_, spanStr := loadFixtures(t, 1)
	var good dbmodel.Span
	require.NoError(t, json.Unmarshal(spanStr, &good))
	badTrace := dbmodel.Trace{Spans: []dbmodel.Span{{TraceID: "wrong-trace-id"}}}
	goodTrace := dbmodel.Trace{Spans: []dbmodel.Span{good}}
	coreReader.On(fxnName, mock.Anything, mock.Anything).
		Return([]dbmodel.Trace{badTrace, goodTrace}, nil)

	var yields int
	for _, err := range actualTraces(reader) {
		yields++
		require.ErrorContains(t, err, "encoding/hex: invalid byte")
		break
	}
	assert.Equal(t, 1, yields, "iteration stops at the break during the error yield")
}

func TestTraceReader_GetTraces_StopsWhenConsumerBreaksOnError(t *testing.T) {
	testStopsWhenConsumerBreaksOnError(t, "GetTraces", func(r TraceReader) iter.Seq2[[]ptrace.Traces, error] {
		return r.GetTraces(context.Background(), tracestore.GetTraceParams{})
	})
}

func TestTraceReader_FindTraces_StopsWhenConsumerBreaksOnError(t *testing.T) {
	testStopsWhenConsumerBreaksOnError(t, "FindTraces", func(r TraceReader) iter.Seq2[[]ptrace.Traces, error] {
		return r.FindTraces(context.Background(), tracestore.TraceQueryParams{
			Attributes: pcommon.NewMap(),
		})
	})
}

func TestTraceReader_FindTraces(t *testing.T) {
	coreReader := &mocks.Reader{}
	reader := TraceReader{spanReader: coreReader}
	tracesStr, spanStr := loadFixtures(t, 1)
	var span dbmodel.Span
	require.NoError(t, json.Unmarshal(spanStr, &span))
	dbTrace := dbmodel.Trace{Spans: []dbmodel.Span{span}}
	span.TraceID = "00000000000000020000000000000000"
	dbTrace2 := dbmodel.Trace{Spans: []dbmodel.Span{span}}
	coreReader.On("FindTraces", mock.Anything, mock.Anything).Return([]dbmodel.Trace{dbTrace, dbTrace2}, nil)
	traces := reader.FindTraces(context.Background(), tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
	})
	for td, err := range traces {
		require.NoError(t, err)
		assert.Len(t, td, 1)
		testTraces(t, tracesStr, td[0])
		break
	}
}

func TestTraceReader_FindTraces_Errors(t *testing.T) {
	testTraceReaderGetTracesAndFindTracesErrors(t, "FindTraces", func(r TraceReader) iter.Seq2[[]ptrace.Traces, error] {
		return r.FindTraces(context.Background(), tracestore.TraceQueryParams{
			Attributes: pcommon.NewMap(),
		})
	})
}

func TestTraceReader_FindTraceIDs(t *testing.T) {
	coreReader := &mocks.Reader{}
	reader := TraceReader{spanReader: coreReader}
	dbTraceIDs := []dbmodel.TraceID{
		"00000000000000010000000000000000",
		"00000000000000020000000000000000",
		"00000000000000030000000000000000",
	}
	expected := make([]tracestore.FoundTraceID, 0, len(dbTraceIDs))
	for _, dbTraceID := range dbTraceIDs {
		expected = append(expected, fromDBTraceId(t, dbTraceID))
	}
	coreReader.On("FindTraceIDs", mock.Anything, mock.Anything).Return(dbTraceIDs, nil)
	for traceIds, err := range reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{
		Attributes: pcommon.NewMap(),
	}) {
		require.NoError(t, err)
		require.Equal(t, expected, traceIds)
	}
}

func TestTraceReader_FindTraceIDs_Error(t *testing.T) {
	tests := []struct {
		name                   string
		errFromCoreReader      error
		traceIdsFromCoreReader []dbmodel.TraceID
		expectedErr            string
	}{
		{
			name:              "some error from core reader",
			errFromCoreReader: errors.New("some error from core reader"),
			expectedErr:       "some error from core reader",
		},
		{
			name:                   "wrong trace id sent from core reader",
			traceIdsFromCoreReader: []dbmodel.TraceID{"wrong-id"},
			expectedErr:            "encoding/hex: invalid byte: U+0077 'w'",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coreReader := &mocks.Reader{}
			attrs := pcommon.NewMap()
			attrs.PutStr("key1", "val1")
			ts := time.Now()
			traceQueryParams := tracestore.TraceQueryParams{
				Attributes:    attrs,
				StartTimeMin:  ts,
				ServiceName:   "testing-service-name",
				OperationName: "testing-operation-name",
				StartTimeMax:  ts.Add(1 * time.Hour),
				DurationMin:   1 * time.Hour,
				DurationMax:   1 * time.Hour,
				SearchDepth:   10,
			}
			dbTraceQueryParams := dbmodel.TraceQueryParameters{
				Tags:          map[string]string{"key1": "val1"},
				StartTimeMin:  ts,
				ServiceName:   "testing-service-name",
				OperationName: "testing-operation-name",
				StartTimeMax:  ts.Add(1 * time.Hour),
				DurationMin:   1 * time.Hour,
				DurationMax:   1 * time.Hour,
				SearchDepth:   10,
			}
			coreReader.On("FindTraceIDs", mock.Anything, dbTraceQueryParams).Return(test.traceIdsFromCoreReader, test.errFromCoreReader)
			reader := TraceReader{spanReader: coreReader}
			for traceIds, err := range reader.FindTraceIDs(context.Background(), traceQueryParams) {
				require.ErrorContains(t, err, test.expectedErr)
				require.Nil(t, traceIds)
			}
		})
	}
}

// TestTraceReader_FindTraceIDs_Filter checks that a structured filter reaches the core
// reader as the expression tree it is. The query service keeps it mutually exclusive with
// the legacy predicate fields, so those travel empty alongside it.
func TestTraceReader_FindTraceIDs_Filter(t *testing.T) {
	filter := &expression.Call{
		Op: expression.OpGt,
		Args: []expression.Expression{
			&expression.FieldRef{Name: expression.SpanFieldDuration, Level: expression.LevelSpan},
			&expression.AnyValue{Value: "2s"},
		},
	}
	ts := time.Now()
	coreReader := &mocks.Reader{}
	coreReader.On("FindTraceIDs", mock.Anything, dbmodel.TraceQueryParameters{
		Tags:         map[string]string{},
		StartTimeMin: ts,
		StartTimeMax: ts.Add(time.Hour),
		Filter:       filter,
	}).Return([]dbmodel.TraceID{}, nil)
	reader := TraceReader{spanReader: coreReader}
	for _, err := range reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{
		Attributes:   pcommon.NewMap(),
		StartTimeMin: ts,
		StartTimeMax: ts.Add(time.Hour),
		Filter:       filter,
	}) {
		require.NoError(t, err)
	}
	coreReader.AssertExpectations(t)
}

func Test_NewTraceReader(t *testing.T) {
	reader := NewTraceReader(core.SpanReaderParams{
		Logger: zap.NewNop(),
	})
	assert.IsType(t, &core.SpanReader{}, reader.spanReader)
}

func fromDBTraceId(t *testing.T, traceID dbmodel.TraceID) tracestore.FoundTraceID {
	traceId, err := convertTraceIDFromDB(traceID)
	require.NoError(t, err)
	return tracestore.FoundTraceID{
		TraceID: traceId,
	}
}
