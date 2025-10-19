// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

func putAttributes(
	t *testing.T,
	attrs pcommon.Map,
	boolKeys []string, boolValues []bool,
	doubleKeys []string, doubleValues []float64,
	intKeys []string, intValues []int64,
	strKeys []string, strValues []string,
	complexKeys []string, complexValues []string,
) {
	t.Helper()
	for i := 0; i < len(boolKeys); i++ {
		attrs.PutBool(boolKeys[i], boolValues[i])
	}
	for i := 0; i < len(doubleKeys); i++ {
		attrs.PutDouble(doubleKeys[i], doubleValues[i])
	}
	for i := 0; i < len(intKeys); i++ {
		attrs.PutInt(intKeys[i], intValues[i])
	}
	for i := 0; i < len(strKeys); i++ {
		attrs.PutStr(strKeys[i], strValues[i])
	}
	for i := 0; i < len(complexKeys); i++ {
		if strings.HasPrefix(complexKeys[i], "@bytes@") {
			decoded, err := base64.StdEncoding.DecodeString(complexValues[i])
			require.NoError(t, err)
			k := strings.TrimPrefix(complexKeys[i], "@bytes@")
			attrs.PutEmptyBytes(k).FromRaw(decoded)
		}
	}
}

func tracesFromSpanRows(t *testing.T, rows []*spanRow) ptrace.Traces {
	td := ptrace.NewTraces()
	for _, r := range rows {
		rs := td.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr(otelsemconv.ServiceNameKey, r.serviceName)

		ss := rs.ScopeSpans().AppendEmpty()
		ss.Scope().SetName(r.scopeName)
		ss.Scope().SetVersion(r.scopeVersion)

		span := ss.Spans().AppendEmpty()
		spanID, err := hex.DecodeString(r.id)
		require.NoError(t, err)
		span.SetSpanID(pcommon.SpanID(spanID))
		traceID, err := hex.DecodeString(r.traceID)
		require.NoError(t, err)
		span.SetTraceID(pcommon.TraceID(traceID))
		span.TraceState().FromRaw(r.traceState)
		if r.parentSpanID != "" {
			parentSpanID, err := hex.DecodeString(r.parentSpanID)
			require.NoError(t, err)
			span.SetParentSpanID(pcommon.SpanID(parentSpanID))
		}
		span.SetName(r.name)
		span.SetKind(jptrace.StringToSpanKind(r.kind))
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(r.startTime))
		span.SetEndTimestamp(pcommon.NewTimestampFromTime(r.startTime.Add(time.Duration(r.rawDuration))))
		span.Status().SetCode(jptrace.StringToStatusCode(r.statusCode))
		span.Status().SetMessage(r.statusMessage)

		putAttributes(
			t,
			span.Attributes(),
			r.boolAttributeKeys, r.boolAttributeValues,
			r.doubleAttributeKeys, r.doubleAttributeValues,
			r.intAttributeKeys, r.intAttributeValues,
			r.strAttributeKeys, r.strAttributeValues,
			r.complexAttributeKeys, r.complexAttributeValues,
		)

		for i, e := range r.eventNames {
			event := span.Events().AppendEmpty()
			event.SetName(e)
			event.SetTimestamp(pcommon.NewTimestampFromTime(r.eventTimestamps[i]))
			putAttributes(
				t,
				event.Attributes(),
				r.eventBoolAttributeKeys[i], r.eventBoolAttributeValues[i],
				r.eventDoubleAttributeKeys[i], r.eventDoubleAttributeValues[i],
				r.eventIntAttributeKeys[i], r.eventIntAttributeValues[i],
				r.eventStrAttributeKeys[i], r.eventStrAttributeValues[i],
				r.eventComplexAttributeKeys[i], r.eventComplexAttributeValues[i],
			)
		}

		for i, l := range r.linkTraceIDs {
			link := span.Links().AppendEmpty()
			traceID, err := hex.DecodeString(l)
			require.NoError(t, err)
			link.SetTraceID(pcommon.TraceID(traceID))
			spanID, err := hex.DecodeString(r.linkSpanIDs[i])
			require.NoError(t, err)
			link.SetSpanID(pcommon.SpanID(spanID))
			link.TraceState().FromRaw(r.linkTraceStates[i])

			putAttributes(
				t,
				link.Attributes(),
				r.linkBoolAttributeKeys[i], r.linkBoolAttributeValues[i],
				r.linkDoubleAttributeKeys[i], r.linkDoubleAttributeValues[i],
				r.linkIntAttributeKeys[i], r.linkIntAttributeValues[i],
				r.linkStrAttributeKeys[i], r.linkStrAttributeValues[i],
				r.linkComplexAttributeKeys[i], r.linkComplexAttributeValues[i],
			)
		}
	}
	return td
}

func TestWriter_Success(t *testing.T) {
	conn := &testDriver{
		t:             t,
		expectedQuery: sql.InsertSpan,
		batch:         &testBatch{t: t},
	}
	w := NewWriter(conn)

	td := tracesFromSpanRows(t, multipleSpans)

	err := w.WriteTraces(context.Background(), td)
	require.NoError(t, err)

	require.True(t, conn.batch.sendCalled)
	require.Len(t, conn.batch.appended, len(multipleSpans))

	for i, expected := range multipleSpans {
		row := conn.batch.appended[i]

		require.Equal(t, expected.id, row[0])                      // SpanID
		require.Equal(t, expected.traceID, row[1])                 // TraceID
		require.Equal(t, expected.traceState, row[2])              // TraceState
		require.Equal(t, expected.parentSpanID, row[3])            // ParentSpanID
		require.Equal(t, expected.name, row[4])                    // Name
		require.Equal(t, strings.ToLower(expected.kind), row[5])   // Kind
		require.Equal(t, expected.startTime, row[6])               // StartTimestamp
		require.Equal(t, expected.statusCode, row[7])              // Status code
		require.Equal(t, expected.statusMessage, row[8])           // Status message
		require.EqualValues(t, expected.rawDuration, row[9])       // Duration
		require.Equal(t, expected.serviceName, row[10])            // Service name
		require.Equal(t, expected.scopeName, row[11])              // Scope name
		require.Equal(t, expected.scopeVersion, row[12])           // Scope version
		require.Equal(t, expected.boolAttributeKeys, row[13])      // Bool attribute keys
		require.Equal(t, expected.boolAttributeValues, row[14])    // Bool attribute values
		require.Equal(t, expected.doubleAttributeKeys, row[15])    // Double attribute keys
		require.Equal(t, expected.doubleAttributeValues, row[16])  // Double attribute values
		require.Equal(t, expected.intAttributeKeys, row[17])       // Int attribute keys
		require.Equal(t, expected.intAttributeValues, row[18])     // Int attribute values
		require.Equal(t, expected.strAttributeKeys, row[19])       // Str attribute keys
		require.Equal(t, expected.strAttributeValues, row[20])     // Str attribute values
		require.Equal(t, expected.complexAttributeKeys, row[21])   // Complex attribute keys
		require.Equal(t, expected.complexAttributeValues, row[22]) // Complex attribute values
		require.Equal(t, expected.eventNames, row[23])             // Event names
		require.Equal(t, expected.eventTimestamps, row[24])        // Event timestamps
		require.Equal(t,
			toTuple(expected.eventBoolAttributeKeys, expected.eventBoolAttributeValues),
			row[25],
		) // Event bool attributes
		require.Equal(t,
			toTuple(expected.eventDoubleAttributeKeys, expected.eventDoubleAttributeValues),
			row[26],
		) // Event double attributes
		require.Equal(t,
			toTuple(expected.eventIntAttributeKeys, expected.eventIntAttributeValues),
			row[27],
		) // Event int attributes
		require.Equal(t,
			toTuple(expected.eventStrAttributeKeys, expected.eventStrAttributeValues),
			row[28],
		) // Event str attributes
		require.Equal(t,
			toTuple(expected.eventComplexAttributeKeys, expected.eventComplexAttributeValues),
			row[29],
		) // Event complex attributes
		require.Equal(t, expected.linkTraceIDs, row[30])    // Link TraceIDs
		require.Equal(t, expected.linkSpanIDs, row[31])     // Link SpanIDs
		require.Equal(t, expected.linkTraceStates, row[32]) // Link TraceStates
		require.Equal(t,
			toTuple(expected.linkBoolAttributeKeys, expected.linkBoolAttributeValues),
			row[33],
		) // Link bool attributes
		require.Equal(t,
			toTuple(expected.linkDoubleAttributeKeys, expected.linkDoubleAttributeValues),
			row[34],
		) // Link double attributes
		require.Equal(t,
			toTuple(expected.linkIntAttributeKeys, expected.linkIntAttributeValues),
			row[35],
		) // Link int attributes
		require.Equal(t,
			toTuple(expected.linkStrAttributeKeys, expected.linkStrAttributeValues),
			row[36],
		) // Link str attributes
		require.Equal(t,
			toTuple(expected.linkComplexAttributeKeys, expected.linkComplexAttributeValues),
			row[37],
		) // Link complex attributes
	}
}

func TestWriter_PrepareBatchError(t *testing.T) {
	conn := &testDriver{
		t:             t,
		expectedQuery: sql.InsertSpan,
		err:           assert.AnError,
		batch:         &testBatch{t: t},
	}
	w := NewWriter(conn)
	err := w.WriteTraces(context.Background(), tracesFromSpanRows(t, multipleSpans))
	require.ErrorContains(t, err, "failed to prepare batch")
	require.ErrorIs(t, err, assert.AnError)
	require.False(t, conn.batch.sendCalled)
}

func TestWriter_AppendBatchError(t *testing.T) {
	conn := &testDriver{
		t:             t,
		expectedQuery: sql.InsertSpan,
		batch:         &testBatch{t: t, appendErr: assert.AnError},
	}
	w := NewWriter(conn)
	err := w.WriteTraces(context.Background(), tracesFromSpanRows(t, multipleSpans))
	require.ErrorContains(t, err, "failed to append span to batch")
	require.ErrorIs(t, err, assert.AnError)
	require.False(t, conn.batch.sendCalled)
}

func TestWriter_SendError(t *testing.T) {
	conn := &testDriver{
		t:             t,
		expectedQuery: sql.InsertSpan,
		batch:         &testBatch{t: t, sendErr: assert.AnError},
	}
	w := NewWriter(conn)
	err := w.WriteTraces(context.Background(), tracesFromSpanRows(t, multipleSpans))
	require.ErrorContains(t, err, "failed to send batch")
	require.ErrorIs(t, err, assert.AnError)
	require.False(t, conn.batch.sendCalled)
}

func TestToTuple(t *testing.T) {
	tests := []struct {
		name     string
		keys     [][]string
		values   [][]int
		expected [][][]any
	}{
		{
			name:     "empty slices",
			keys:     [][]string{},
			values:   [][]int{},
			expected: [][][]any{},
		},
		{
			name:     "single empty inner slice",
			keys:     [][]string{{}},
			values:   [][]int{{}},
			expected: [][][]any{{}},
		},
		{
			name:   "single element",
			keys:   [][]string{{"key1"}},
			values: [][]int{{42}},
			expected: [][][]any{
				{
					{"key1", 42},
				},
			},
		},
		{
			name:   "multiple elements in single slice",
			keys:   [][]string{{"key1", "key2", "key3"}},
			values: [][]int{{10, 20, 30}},
			expected: [][][]any{
				{
					{"key1", 10},
					{"key2", 20},
					{"key3", 30},
				},
			},
		},
		{
			name:   "multiple slices with multiple elements",
			keys:   [][]string{{"key1", "key2"}, {"key3"}, {"key4", "key5", "key6"}},
			values: [][]int{{1, 2}, {3}, {4, 5, 6}},
			expected: [][][]any{
				{
					{"key1", 1},
					{"key2", 2},
				},
				{
					{"key3", 3},
				},
				{
					{"key4", 4},
					{"key5", 5},
					{"key6", 6},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toTuple(tt.keys, tt.values)
			require.Equal(t, tt.expected, result)
		})
	}
}
