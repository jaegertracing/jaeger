// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

// This file holds the helpers behind TestKafkaStorage_SyncElasticsearch_FaultInjection
// in e2e_kafka_test.go: a reverse proxy that injects faults into the Elasticsearch
// write path, a Kafka admin reader for consumer-group offsets, and the steps the
// subtests share.

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

// esFault selects how the fault-injecting proxy treats _bulk requests. Every other
// request (version detection, index creation, queries) always passes through, so
// the fault stays confined to the write path under test.
type esFault int32

const (
	// esFaultNone forwards every request to Elasticsearch unchanged.
	esFaultNone esFault = iota
	// esFaultReject answers _bulk with 503 without forwarding it, so nothing is
	// written. It stands in for a backend that is down or overloaded.
	esFaultReject
	// esFaultLoseAck forwards _bulk to Elasticsearch, so the documents are written,
	// but replaces the response with a 503. It stands in for an acknowledgement
	// lost in transit: the ingester must retry, and the retry must not duplicate.
	esFaultLoseAck
)

// esFaultProxy is a reverse proxy in front of Elasticsearch whose treatment of
// _bulk requests the test switches at runtime.
type esFaultProxy struct {
	server *httptest.Server
	mode   atomic.Int32
}

func newESFaultProxy(t *testing.T, target string) *esFaultProxy {
	targetURL, err := url.Parse(target)
	require.NoError(t, err)
	p := &esFaultProxy{}
	rp := httputil.NewSingleHostReverseProxy(targetURL)
	rp.ModifyResponse = func(resp *http.Response) error {
		if p.current() == esFaultLoseAck && isBulk(resp.Request) {
			injectServiceUnavailable(resp)
		}
		return nil
	}
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p.current() == esFaultReject && isBulk(r) {
			http.Error(w, `{"error":"injected by esFaultProxy"}`, http.StatusServiceUnavailable)
			return
		}
		rp.ServeHTTP(w, r)
	}))
	t.Cleanup(p.server.Close)
	return p
}

func (p *esFaultProxy) URL() string { return p.server.URL }

func (p *esFaultProxy) set(mode esFault) { p.mode.Store(int32(mode)) }

func (p *esFaultProxy) current() esFault { return esFault(p.mode.Load()) }

func isBulk(r *http.Request) bool {
	return r != nil && strings.HasSuffix(r.URL.Path, "/_bulk")
}

// injectServiceUnavailable rewrites an upstream response into a 503 after the
// upstream has already applied the request.
func injectServiceUnavailable(resp *http.Response) {
	resp.Body.Close()
	body := `{"error":"acknowledgement dropped by esFaultProxy"}`
	resp.StatusCode = http.StatusServiceUnavailable
	resp.Status = http.StatusText(http.StatusServiceUnavailable)
	resp.Body = io.NopCloser(strings.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
	resp.Header.Set("Content-Type", "application/json")
}

// kafkaOffsets reads the consumer group's committed offset and the log end offset
// for one topic, summed over its partitions. The test asserts on the gap between
// the two: it must not close while writes fail and must close once they succeed.
type kafkaOffsets struct {
	admin *kadm.Client
	group string
	topic string
}

func newKafkaOffsets(t *testing.T, broker, group, topic string) *kafkaOffsets {
	client, err := kgo.NewClient(kgo.SeedBrokers(broker))
	require.NoError(t, err)
	t.Cleanup(client.Close)
	return &kafkaOffsets{admin: kadm.NewClient(client), group: group, topic: topic}
}

func (k *kafkaOffsets) committed(t *testing.T) int64 {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := k.admin.FetchOffsetsForTopics(ctx, k.group, k.topic)
	require.NoError(t, err)
	var total int64
	for _, partitions := range resp {
		for _, o := range partitions {
			require.NoError(t, o.Err)
			if o.At > 0 {
				total += o.At
			}
		}
	}
	return total
}

func (k *kafkaOffsets) end(t *testing.T) int64 {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := k.admin.ListEndOffsets(ctx, k.topic)
	require.NoError(t, err)
	var total int64
	for _, partitions := range resp {
		for _, o := range partitions {
			require.NoError(t, o.Err)
			total += o.Offset
		}
	}
	return total
}

// faultInjectionSteps groups the pipeline handles the fault-injection subtests share.
type faultInjectionSteps struct {
	collector *E2EStorageIntegration
	ingester  *E2EStorageIntegration
	proxy     *esFaultProxy
	offsets   *kafkaOffsets
}

// outageHoldTime is how long a fault stays injected after the message has landed
// in Kafka. It exceeds the exporter's retry interval and the receiver's 1s
// autocommit interval several times over, so an offset that was going to advance
// wrongly would have done so.
const outageHoldTime = 15 * time.Second

// runOutage injects one fault, writes a trace through it, checks that the committed
// offset holds while the fault lasts, then lifts the fault and checks that the trace
// is stored exactly once and the offset catches up.
func (f *faultInjectionSteps) runOutage(t *testing.T, fault esFault, traceIDByte byte, duringOutage func(t *testing.T, trace ptrace.Traces)) {
	f.requireOffsetCaughtUp(t)
	committedBefore := f.offsets.committed(t)
	endBefore := f.offsets.end(t)

	f.proxy.set(fault)
	defer f.proxy.set(esFaultNone)
	trace := f.write(t, traceIDByte, 9)

	require.Eventually(t, func() bool { return f.offsets.end(t) > endBefore },
		time.Minute, time.Second, "the trace never reached Kafka")
	t.Logf("Trace is in Kafka; holding the fault for %v", outageHoldTime)
	time.Sleep(outageHoldTime)

	assert.Equal(t, committedBefore, f.offsets.committed(t), "the committed offset must not advance while the write fails")
	duringOutage(t, trace)

	f.proxy.set(esFaultNone)
	t.Log("Fault lifted; waiting for recovery")
	f.requireStoredOnce(t, trace)
	f.requireOffsetCaughtUp(t)
}

// write sends a trace with spanCount spans into the collector and returns it.
func (f *faultInjectionSteps) write(t *testing.T, traceIDByte byte, spanCount int) ptrace.Traces {
	trace := buildFaultInjectionTrace(traceIDByte, spanCount)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	require.NoError(t, f.collector.TraceWriter.WriteTraces(ctx, trace))
	return trace
}

// storedSpanCount returns how many spans of the trace Elasticsearch holds right now.
func (f *faultInjectionSteps) storedSpanCount(t *testing.T, trace ptrace.Traces) int {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	seq := f.ingester.TraceReader.GetTraces(ctx, tracestore.GetTraceParams{TraceID: jptrace.GetTraceID(trace)})
	traces, err := jiter.CollectWithErrors(jptrace.AggregateTraces(seq))
	require.NoError(t, err)
	total := 0
	for _, tr := range traces {
		total += tr.SpanCount()
	}
	return total
}

// requireStoredOnce waits until every span of the trace is readable and then checks
// that none is stored twice. A duplicated write shows up as an extra document with
// its own _id, which the reader returns as an extra span.
func (f *faultInjectionSteps) requireStoredOnce(t *testing.T, trace ptrace.Traces) {
	expected := trace.SpanCount()
	require.Eventually(t, func() bool { return f.storedSpanCount(t, trace) >= expected },
		2*time.Minute, time.Second, "trace %s never became fully readable", jptrace.GetTraceID(trace))
	// Give a duplicate written after the last expected span a chance to become
	// visible before counting.
	time.Sleep(2 * time.Second)
	assert.Equal(t, expected, f.storedSpanCount(t, trace), "trace %s must be stored exactly once", jptrace.GetTraceID(trace))
}

// requireOffsetCaughtUp waits until the committed offset equals the end offset.
func (f *faultInjectionSteps) requireOffsetCaughtUp(t *testing.T) {
	require.Eventually(t, func() bool {
		committed, end := f.offsets.committed(t), f.offsets.end(t)
		t.Logf("Kafka offsets: committed=%d end=%d", committed, end)
		return end > 0 && committed == end
	}, 2*time.Minute, time.Second, "the committed offset never caught up with the end offset")
}

// buildFaultInjectionTrace returns one trace of spanCount distinct spans. The trace
// ID combines traceIDByte, which tells the subtests apart, with the current time,
// so a rerun against a backend still holding an earlier run's documents does not
// count them.
func buildFaultInjectionTrace(traceIDByte byte, spanCount int) ptrace.Traces {
	now := time.Now()
	var traceID pcommon.TraceID
	traceID[0] = traceIDByte
	binary.BigEndian.PutUint64(traceID[8:], uint64(now.UnixNano()))

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr(otelsemconv.ServiceNameKey, "fault-injection")
	spans := rs.ScopeSpans().AppendEmpty().Spans()
	for i := range spanCount {
		span := spans.AppendEmpty()
		span.SetTraceID(traceID)
		var spanID pcommon.SpanID
		binary.BigEndian.PutUint64(spanID[:], uint64(i+1))
		span.SetSpanID(spanID)
		span.SetName(fmt.Sprintf("op-%d", i))
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(now.Add(time.Duration(i) * time.Millisecond)))
		span.SetEndTimestamp(pcommon.NewTimestampFromTime(now.Add(time.Duration(i+1) * time.Millisecond)))
	}
	return td
}

// kafkaBroker is the broker the collector and ingester configs default to.
func kafkaBroker() string {
	return "localhost:9092"
}
