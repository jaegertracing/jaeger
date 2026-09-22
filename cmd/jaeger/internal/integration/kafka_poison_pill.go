// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

// This file holds the helpers behind TestKafkaStorage_SyncElasticsearch_DeadLetter
// and TestKafkaStorage_SyncElasticsearch_PoisonDrop in e2e_kafka_test.go: a poison
// trace Elasticsearch rejects deterministically, and an OTLP/HTTP server that stands
// in for the dead-letter sink and records what the pipeline sends it.

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
)

// poisonSpanFlags is a span flags value outside the range of the `flags` field's
// `integer` mapping in the Jaeger span index template (a signed 32-bit int), so
// Elasticsearch rejects the document with a mapper_parsing_exception (status 400)
// on every attempt. That makes the span a poison pill by construction rather than
// by faking a response: the write path is real from the collector to the backend.
const poisonSpanFlags uint32 = 1 << 31

// buildPoisonTrace returns a trace like buildFaultInjectionTrace's whose first span
// is a poison pill, along with that span's id.
func buildPoisonTrace(traceIDByte byte, spanCount int) (ptrace.Traces, pcommon.SpanID) {
	trace := buildFaultInjectionTrace(traceIDByte, spanCount)
	poison := trace.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	poison.SetFlags(poisonSpanFlags)
	return trace, poison.SpanID()
}

// deadLetterServer is an OTLP/HTTP traces endpoint that records every span the
// dead-letter pipeline exports to it.
type deadLetterServer struct {
	server *httptest.Server
	mu     sync.Mutex
	spans  []ptrace.Span
}

func newDeadLetterServer(t *testing.T) *deadLetterServer {
	d := &deadLetterServer{}
	d.server = httptest.NewServer(http.HandlerFunc(d.handle))
	t.Cleanup(d.server.Close)
	return d
}

// TracesURL is the endpoint for an otlphttp exporter's traces_endpoint.
func (d *deadLetterServer) TracesURL() string { return d.server.URL + "/v1/traces" }

func (d *deadLetterServer) handle(w http.ResponseWriter, r *http.Request) {
	body := io.Reader(r.Body)
	if r.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer gz.Close()
		body = gz
	}
	raw, err := io.ReadAll(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req := ptraceotlp.NewExportRequest()
	if err := req.UnmarshalProto(raw); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	d.mu.Lock()
	for _, rs := range req.Traces().ResourceSpans().All() {
		for _, ss := range rs.ScopeSpans().All() {
			for _, span := range ss.Spans().All() {
				copied := ptrace.NewSpan()
				span.CopyTo(copied)
				d.spans = append(d.spans, copied)
			}
		}
	}
	d.mu.Unlock()
	resp, err := ptraceotlp.NewExportResponse().MarshalProto()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	_, _ = w.Write(resp)
}

// spans returns a copy of every span received so far.
func (d *deadLetterServer) received() []ptrace.Span {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]ptrace.Span, len(d.spans))
	copy(out, d.spans)
	return out
}

// writePoison sends a trace whose first span is a poison pill into the collector,
// waits for it to land in Kafka, and returns the trace and the poison span's id.
// Waiting for the record matters because requireOffsetCaughtUp would otherwise be
// satisfied by the offsets as they stood before the write.
func (f *faultInjectionSteps) writePoison(t *testing.T, traceIDByte byte) (ptrace.Traces, pcommon.SpanID) {
	logEndBefore := requireOffsets(t, f.offsets.logEnd)
	trace, poisonSpanID := buildPoisonTrace(traceIDByte, MaxChunkSize)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	require.NoError(t, f.collector.TraceWriter.WriteTraces(ctx, trace))
	require.Eventually(t, func() bool {
		logEnd, err := f.offsets.logEnd()
		return err == nil && logEnd.advancedPast(logEndBefore)
	}, time.Minute, time.Second, "the poison trace never reached Kafka")
	return trace, poisonSpanID
}

// requirePoisonStoredAround waits until the offset has caught up past the poison
// trace's record and then checks that every span of the trace except the poison one
// is stored exactly once: the batch completed around the poison pill instead of
// stalling on it.
func (f *faultInjectionSteps) requirePoisonStoredAround(t *testing.T, trace ptrace.Traces) {
	f.requireOffsetCaughtUp(t)
	expected := trace.SpanCount() - 1
	id := singleTraceID(trace)
	require.Eventually(t, func() bool {
		count, err := f.countStored(id)
		return err == nil && count >= expected
	}, 2*time.Minute, time.Second, "the non-poison spans of trace %s never became fully indexed", id)
	require.Equal(t, expected, f.storedSpanCount(t, trace), "trace %s must be stored once without its poison span", id)
}
