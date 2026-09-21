// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/internal/jptrace"
)

// fakeES stands in for Elasticsearch behind the fault proxy. It answers every
// request with 200 and a gzip Content-Encoding header, the way a real backend
// answers a client that asked for compression, and counts the requests it saw.
func fakeES(t *testing.T) (*httptest.Server, *atomic.Int32) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"upstream":true}`))
	}))
	t.Cleanup(server.Close)
	return server, &hits
}

func postBulk(t *testing.T, proxy *esFaultProxy, path string) *http.Response {
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, proxy.URL()+path, strings.NewReader("{}\n"))
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestESFaultProxy_PassesThroughByDefault(t *testing.T) {
	backend, hits := fakeES(t)
	proxy := newESFaultProxy(t, backend.URL)

	resp := postBulk(t, proxy, "/_bulk")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(1), hits.Load())
}

func TestESFaultProxy_RejectAnswersBulkWithoutForwarding(t *testing.T) {
	backend, hits := fakeES(t)
	proxy := newESFaultProxy(t, backend.URL)
	proxy.setFault(esFaultReject)

	resp := postBulk(t, proxy, "/_bulk")
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	assert.JSONEq(t, `{"error":"injected by esFaultProxy"}`, string(body))
	assert.Zero(t, hits.Load(), "a rejected _bulk must not reach the backend")

	// Only _bulk is faulted; everything else still passes through.
	other := postBulk(t, proxy, "/_cluster/health")
	assert.Equal(t, http.StatusOK, other.StatusCode)
	assert.Equal(t, int32(1), hits.Load())
}

func TestESFaultProxy_LoseAckForwardsThenFails(t *testing.T) {
	backend, hits := fakeES(t)
	proxy := newESFaultProxy(t, backend.URL)
	proxy.setFault(esFaultLoseAck)

	resp := postBulk(t, proxy, "/_bulk")
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, int32(1), hits.Load(), "the backend must apply the request before the ack is dropped")
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.JSONEq(t, `{"error":"acknowledgement dropped by esFaultProxy"}`, string(body))
	assert.Empty(t, resp.Header.Get("Content-Encoding"), "the replacement body is not compressed")

	proxy.setFault(esFaultNone)
	resp = postBulk(t, proxy, "/_bulk")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestPartitionOffsets_AdvancedPast(t *testing.T) {
	before := partitionOffsets{0: 5, 1: 7}
	assert.False(t, partitionOffsets{0: 5, 1: 7}.advancedPast(before), "unchanged")
	assert.False(t, partitionOffsets{0: 4, 1: 7}.advancedPast(before), "lower is not an advance")
	assert.True(t, partitionOffsets{0: 5, 1: 8}.advancedPast(before), "one partition moved")
	assert.True(t, partitionOffsets{2: 1}.advancedPast(before), "a new partition counts from zero")
	assert.False(t, partitionOffsets{}.advancedPast(partitionOffsets{}), "nothing to compare")
}

func TestBuildFaultInjectionTrace(t *testing.T) {
	trace := buildFaultInjectionTrace(0x2a, 4)
	require.Equal(t, 4, trace.SpanCount())

	id := singleTraceID(trace)
	assert.Equal(t, byte(0x2a), id[0], "the first byte tells the subtests apart")
	assert.NotEqual(t, pcommon.NewTraceIDEmpty(), id)

	spanIDs := map[pcommon.SpanID]struct{}{}
	for _, span := range jptrace.SpanIter(trace) {
		assert.Equal(t, id, span.TraceID(), "every span belongs to the one trace")
		spanIDs[span.SpanID()] = struct{}{}
	}
	assert.Len(t, spanIDs, 4, "span IDs are distinct")

	again := buildFaultInjectionTrace(0x2a, 4)
	assert.NotEqual(t, id, singleTraceID(again), "a later build gets a different trace ID")
}

func TestKafkaBroker(t *testing.T) {
	t.Setenv("KAFKA_BROKER", "")
	assert.Equal(t, "localhost:9092", kafkaBroker())
	t.Setenv("KAFKA_BROKER", "broker.internal:9093")
	assert.Equal(t, "broker.internal:9093", kafkaBroker())
}
