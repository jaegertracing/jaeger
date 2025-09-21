// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

// TODO: move to JSON fixture
func makeTestTraces() ptrace.Traces {
	td := ptrace.NewTraces()
	fixedTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// ---------- Span 1 ----------
	rs1 := td.ResourceSpans().AppendEmpty()
	rs1.Resource().Attributes().PutStr(otelsemconv.ServiceNameKey, "user-service")

	ss1 := rs1.ScopeSpans().AppendEmpty()
	ss1.Scope().SetName("auth-scope")
	ss1.Scope().SetVersion("v1.0.0")

	span1 := ss1.Spans().AppendEmpty()
	span1.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1}))
	span1.SetTraceID(pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}))
	span1.TraceState().FromRaw("state1")
	span1.SetName("GET /api/user")
	span1.SetKind(ptrace.SpanKindServer)
	start1 := pcommon.NewTimestampFromTime(fixedTime)
	end1 := pcommon.NewTimestampFromTime(start1.AsTime().Add(time.Second))
	span1.SetStartTimestamp(start1)
	span1.SetEndTimestamp(end1)
	span1.Status().SetCode(ptrace.StatusCodeOk)
	span1.Status().SetMessage("success")

	span1.Attributes().PutBool("authenticated", true)
	span1.Attributes().PutBool("cache_hit", false)
	span1.Attributes().PutDouble("response_time", 0.123)
	span1.Attributes().PutDouble("cpu_usage", 45.67)
	span1.Attributes().PutInt("user_id", 12345)
	span1.Attributes().PutInt("request_size", 1024)
	span1.Attributes().PutStr("http.method", "GET")
	span1.Attributes().PutStr("http.url", "/api/user")
	span1.Attributes().PutEmptyBytes("@bytes@request_body").FromRaw([]byte(`{"name":"test"}`))

	ev1 := span1.Events().AppendEmpty()
	ev1.SetName("login")
	ev1.SetTimestamp(pcommon.NewTimestampFromTime(fixedTime.Add(-time.Second)))
	ev1.Attributes().PutBool("login_successful", true)
	ev1.Attributes().PutDouble("response_time", 0.123)
	ev1.Attributes().PutInt("attempt_count", 1)
	ev1.Attributes().PutStr("user_agent", "Mozilla/5.0")
	ev1.Attributes().PutEmptyBytes("@bytes@login_data").FromRaw([]byte(`{"login":"true"}`))

	link1 := span1.Links().AppendEmpty()
	link1.SetTraceID(pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}))
	link1.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 2}))
	link1.TraceState().FromRaw("state2")

	// ---------- Span 2 ----------
	rs2 := td.ResourceSpans().AppendEmpty()
	rs2.Resource().Attributes().PutStr(otelsemconv.ServiceNameKey, "order-service")

	ss2 := rs2.ScopeSpans().AppendEmpty()
	ss2.Scope().SetName("checkout-scope")
	ss2.Scope().SetVersion("v1.1.0")

	span2 := ss2.Spans().AppendEmpty()
	span2.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 2}))
	span2.SetTraceID(pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}))
	span2.TraceState().FromRaw("state2")
	span2.SetParentSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1}))
	span2.SetName("POST /api/order")
	span2.SetKind(ptrace.SpanKindServer)
	start2 := pcommon.NewTimestampFromTime(fixedTime)
	end2 := pcommon.NewTimestampFromTime(start2.AsTime().Add(2500 * time.Millisecond))
	span2.SetStartTimestamp(start2)
	span2.SetEndTimestamp(end2)
	span2.Status().SetCode(ptrace.StatusCodeOk)
	span2.Status().SetMessage("success")

	span2.Attributes().PutBool("payment_successful", true)
	span2.Attributes().PutBool("idempotent", true)
	span2.Attributes().PutDouble("checkout_time", 1.234)
	span2.Attributes().PutDouble("memory_usage", 78.9)
	span2.Attributes().PutInt("order_id", 98765)
	span2.Attributes().PutInt("items_count", 3)
	span2.Attributes().PutStr("http.method", "POST")
	span2.Attributes().PutStr("db.system", "mysql")
	span2.Attributes().PutEmptyBytes("@bytes@order_payload").FromRaw([]byte(`{"items":["book","checkout"]}`))

	ev2a := span2.Events().AppendEmpty()
	ev2a.SetName("checkout")
	ev2a.SetTimestamp(pcommon.NewTimestampFromTime(fixedTime.Add(-2 * time.Second)))
	ev2a.Attributes().PutBool("payment_verified", true)
	ev2a.Attributes().PutDouble("amount", 199.99)
	ev2a.Attributes().PutInt("transaction_id", 78901)
	ev2a.Attributes().PutStr("payment_method", "credit_card")
	ev2a.Attributes().PutEmptyBytes("@bytes@receipt").FromRaw([]byte(`{"receipt":"valid"}`))

	ev2b := span2.Events().AppendEmpty()
	ev2b.SetName("payment")
	ev2b.SetTimestamp(pcommon.NewTimestampFromTime(fixedTime.Add(-time.Second)))
	ev2b.Attributes().PutBool("transaction_complete", true)
	ev2b.Attributes().PutDouble("processing_fee", 2.99)
	ev2b.Attributes().PutInt("merchant_id", 456)
	ev2b.Attributes().PutStr("currency", "USD")
	ev2b.Attributes().PutEmptyBytes("@bytes@confirmation").FromRaw([]byte(`{"status":"complete"}`))

	link2a := span2.Links().AppendEmpty()
	link2a.SetTraceID(pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}))
	link2a.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1}))
	link2a.TraceState().FromRaw("state1")

	link2b := span2.Links().AppendEmpty()
	link2b.SetTraceID(pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3}))
	link2b.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 3}))
	link2b.TraceState().FromRaw("state1")

	// ---------- Span 3 ----------
	rs3 := td.ResourceSpans().AppendEmpty()
	rs3.Resource().Attributes().PutStr(otelsemconv.ServiceNameKey, "frontend")

	ss3 := rs3.ScopeSpans().AppendEmpty()
	ss3.Scope().SetName("web-scope")
	ss3.Scope().SetVersion("v2.0.0")

	span3 := ss3.Spans().AppendEmpty()
	span3.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 3}))
	span3.SetTraceID(pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3}))
	span3.TraceState().FromRaw("state1")
	span3.SetName("GET /api/user")
	span3.SetKind(ptrace.SpanKindClient)
	start3 := pcommon.NewTimestampFromTime(fixedTime)
	end3 := pcommon.NewTimestampFromTime(start3.AsTime().Add(500 * time.Millisecond))
	span3.SetStartTimestamp(start3)
	span3.SetEndTimestamp(end3)
	span3.Status().SetCode(ptrace.StatusCodeError)
	span3.Status().SetMessage("timeout")

	span3.Attributes().PutBool("retry_attempted", true)
	span3.Attributes().PutBool("cached_response", false)
	span3.Attributes().PutDouble("latency", 0.5)
	span3.Attributes().PutDouble("error_rate", 99.9)
	span3.Attributes().PutInt("retry_count", 2)
	span3.Attributes().PutInt("timeout_ms", 5000)
	span3.Attributes().PutStr("error.type", "TimeoutError")
	span3.Attributes().PutStr("component", "frontend")
	span3.Attributes().PutEmptyBytes("@bytes@response_snippet").FromRaw([]byte(`{"error":"timeout"}`))

	ev3 := span3.Events().AppendEmpty()
	ev3.SetName("fetch")
	ev3.SetTimestamp(pcommon.NewTimestampFromTime(fixedTime.Add(-500 * time.Millisecond)))
	ev3.Attributes().PutBool("fetch_failed", true)
	ev3.Attributes().PutDouble("timeout_duration", 5.0)
	ev3.Attributes().PutInt("status_code", 408)
	ev3.Attributes().PutStr("error_message", "Request timeout")
	ev3.Attributes().PutEmptyBytes("@bytes@error_details").FromRaw([]byte(`{"error":"timeout","code":408}`))

	return td
}

func TestWriter_Success(t *testing.T) {
	tb := &testBatch{t: t}
	conn := &testDriver{
		t:             t,
		expectedQuery: sql.SpansInsert,
		batch:         tb,
	}
	w := NewWriter(conn)

	err := w.WriteTraces(context.Background(), makeTestTraces())
	require.NoError(t, err)

	// Ensure Send was called
	require.True(t, tb.sendCalled)

	// Ensure exactly 3 spans were appended
	require.Len(t, tb.appended, 3)

	fixedTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// ---------- Span 1 ----------
	s1 := tb.appended[0]
	require.Len(t, s1, 13)
	require.Equal(t, "0000000000000001", s1[0])                 // SpanID
	require.Equal(t, "00000000000000000000000000000001", s1[1]) // TraceID
	require.Equal(t, "state1", s1[2])                           // TraceState
	require.Empty(t, s1[3])                                     // ParentSpanID
	require.Equal(t, "GET /api/user", s1[4])                    // Name
	require.EqualValues(t, ptrace.SpanKindServer, s1[5])        // Kind
	require.Equal(t, fixedTime, s1[6])                          // StartTimestamp
	require.EqualValues(t, ptrace.StatusCodeOk, s1[7])          // Status code
	require.Equal(t, "success", s1[8])                          // Status message
	require.EqualValues(t, int64(time.Second), s1[9])           // Duration ≈ 1s
	require.Equal(t, "user-service", s1[10])                    // Service name
	require.Equal(t, "auth-scope", s1[11])                      // Scope name
	require.Equal(t, "v1.0.0", s1[12])                          // Scope version

	// ---------- Span 2 ----------
	s2 := tb.appended[1]
	require.Len(t, s2, 13)
	require.Equal(t, "0000000000000002", s2[0])
	require.Equal(t, "00000000000000000000000000000002", s2[1])
	require.Equal(t, "state2", s2[2])
	require.Equal(t, "0000000000000001", s2[3])
	require.Equal(t, "POST /api/order", s2[4])
	require.EqualValues(t, ptrace.SpanKindServer, s2[5])
	require.Equal(t, fixedTime, s2[6])
	require.EqualValues(t, ptrace.StatusCodeOk, s2[7])
	require.Equal(t, "success", s2[8])
	require.EqualValues(t, int64(2500*time.Millisecond), s2[9])
	require.Equal(t, "order-service", s2[10])
	require.Equal(t, "checkout-scope", s2[11])
	require.Equal(t, "v1.1.0", s2[12])

	// ---------- Span 3 ----------
	s3 := tb.appended[2]
	require.Len(t, s3, 13)
	require.Equal(t, "0000000000000003", s3[0])
	require.Equal(t, "00000000000000000000000000000003", s3[1])
	require.Equal(t, "state1", s3[2])
	require.Empty(t, s3[3])
	require.Equal(t, "GET /api/user", s3[4])
	require.EqualValues(t, ptrace.SpanKindClient, s3[5])
	require.Equal(t, fixedTime, s3[6])
	require.EqualValues(t, ptrace.StatusCodeError, s3[7])
	require.Equal(t, "timeout", s3[8])
	require.EqualValues(t, int64(500*time.Millisecond), s3[9])
	require.Equal(t, "frontend", s3[10])
	require.Equal(t, "web-scope", s3[11])
	require.Equal(t, "v2.0.0", s3[12])
}

func TestWriter_PrepareBatchError(t *testing.T) {
	conn := &testDriver{
		t:             t,
		expectedQuery: sql.SpansInsert,
		err:           assert.AnError,
		batch:         &testBatch{t: t},
	}
	w := NewWriter(conn)
	err := w.WriteTraces(context.Background(), makeTestTraces())
	require.ErrorContains(t, err, "failed to prepare batch")
	require.ErrorIs(t, err, assert.AnError)
}

func TestWriter_AppendBatchError(t *testing.T) {
	conn := &testDriver{
		t:             t,
		expectedQuery: sql.SpansInsert,
		batch:         &testBatch{t: t, appendErr: assert.AnError},
	}
	w := NewWriter(conn)
	err := w.WriteTraces(context.Background(), makeTestTraces())
	require.ErrorContains(t, err, "failed to append span to batch")
	require.ErrorIs(t, err, assert.AnError)
}

func TestWriter_SendError(t *testing.T) {
	conn := &testDriver{
		t:             t,
		expectedQuery: sql.SpansInsert,
		batch:         &testBatch{t: t, sendErr: assert.AnError},
	}
	w := NewWriter(conn)
	err := w.WriteTraces(context.Background(), makeTestTraces())
	require.ErrorContains(t, err, "failed to send batch")
	require.ErrorIs(t, err, assert.AnError)
}
