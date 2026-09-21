// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

// This file holds the helpers behind TestKafkaStorage_SyncElasticsearch_FaultInjection
// in e2e_kafka_test.go: a reverse proxy that injects faults into the Elasticsearch
// write path, a Kafka admin reader for consumer-group offsets, a document counter
// that reads Elasticsearch directly, and the steps the subtests share.

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
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
	server   *httptest.Server
	injected atomic.Int32 // the esFault currently applied to _bulk requests
}

func newESFaultProxy(t *testing.T, target string) *esFaultProxy {
	targetURL, err := url.Parse(target)
	require.NoError(t, err)
	p := &esFaultProxy{}
	rp := httputil.NewSingleHostReverseProxy(targetURL)
	rp.ModifyResponse = func(resp *http.Response) error {
		if p.fault() == esFaultLoseAck && isBulk(resp.Request) {
			injectServiceUnavailable(resp)
		}
		return nil
	}
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p.fault() == esFaultReject && isBulk(r) {
			http.Error(w, `{"error":"injected by esFaultProxy"}`, http.StatusServiceUnavailable)
			return
		}
		rp.ServeHTTP(w, r)
	}))
	t.Cleanup(p.server.Close)
	return p
}

func (p *esFaultProxy) URL() string { return p.server.URL }

func (p *esFaultProxy) setFault(fault esFault) { p.injected.Store(int32(fault)) }

func (p *esFaultProxy) fault() esFault { return esFault(p.injected.Load()) }

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
	// The upstream body was gzip-compressed; the replacement is not.
	resp.Header.Del("Content-Encoding")
}

// kafkaBroker returns the broker the collector and ingester configs connect to,
// read from the same KAFKA_BROKER variable with the same default.
func kafkaBroker() string {
	if broker, ok := os.LookupEnv("KAFKA_BROKER"); ok && broker != "" {
		return broker
	}
	return "localhost:9092"
}

// kafkaOffsets reads, per partition of one topic, the consumer group's committed
// offset and the log end offset. The test asserts on the gap between the two: it
// must not close while writes fail and must close once they succeed.
//
// The readers return errors instead of failing the test so they can run inside a
// require.Eventually condition, which testify evaluates on its own goroutine where
// FailNow is not allowed. A transient admin error there just means "not yet".
type kafkaOffsets struct {
	admin *kadm.Client
	group string
	topic string
}

// partitionOffsets maps a partition to an offset. A partition the group has never
// committed for is reported at 0, which is where its consumption starts.
type partitionOffsets map[int32]int64

func newKafkaOffsets(t *testing.T, broker, group, topic string) *kafkaOffsets {
	client, err := kgo.NewClient(kgo.SeedBrokers(broker))
	require.NoError(t, err)
	t.Cleanup(client.Close)
	return &kafkaOffsets{admin: kadm.NewClient(client), group: group, topic: topic}
}

func (k *kafkaOffsets) committed() (partitionOffsets, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := k.admin.FetchOffsetsForTopics(ctx, k.group, k.topic)
	if err != nil {
		return nil, err
	}
	offsets := partitionOffsets{}
	for _, o := range resp[k.topic] {
		if o.Err != nil {
			return nil, o.Err
		}
		// kadm reports -1 for a partition without a committed offset.
		offsets[o.Partition] = max(o.At, 0)
	}
	return offsets, nil
}

func (k *kafkaOffsets) end() (partitionOffsets, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := k.admin.ListEndOffsets(ctx, k.topic)
	if err != nil {
		return nil, err
	}
	offsets := partitionOffsets{}
	for _, o := range resp[k.topic] {
		if o.Err != nil {
			return nil, o.Err
		}
		offsets[o.Partition] = o.Offset
	}
	return offsets, nil
}

// advancedPast reports whether any partition's offset is higher than in before.
func (o partitionOffsets) advancedPast(before partitionOffsets) bool {
	for partition, offset := range o {
		if offset > before[partition] {
			return true
		}
	}
	return false
}

// countSpanDocs asks Elasticsearch directly how many span documents carry the
// trace ID. It bypasses the query service on purpose: jaeger_query deduplicates
// spans before returning a trace, which would hide a document written twice.
// The index pattern follows the index_prefix in config-kafka-ingester-sync.yaml.
func countSpanDocs(ctx context.Context, traceID pcommon.TraceID) (int, error) {
	query := fmt.Sprintf(`{"query":{"term":{"traceID":%q}}}`, traceID.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		esBaseURL+"/jaeger-main-jaeger-span-*/_count", bytes.NewBufferString(query))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("_count returned %d: %s", resp.StatusCode, body)
	}
	var result struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}
	return result.Count, nil
}

// faultInjectionSteps groups the pipeline handles the fault-injection subtests share.
type faultInjectionSteps struct {
	collector *E2EStorageIntegration
	proxy     *esFaultProxy
	offsets   *kafkaOffsets
}

// outageHoldTime is how long a fault stays injected after the message has landed
// in Kafka. It exceeds the exporter's retry interval and the receiver's 1s
// autocommit interval several times over, so an offset that was going to advance
// wrongly would have done so.
const outageHoldTime = 15 * time.Second

// runOutage injects one fault, writes a trace through it, checks that the committed
// offset holds while the fault lasts, then lifts the fault and checks that the offset
// catches up and the trace is stored exactly once.
func (f *faultInjectionSteps) runOutage(t *testing.T, fault esFault, traceIDByte byte, duringOutage func(t *testing.T, trace ptrace.Traces)) {
	f.requireOffsetCaughtUp(t)
	committedBefore := f.requireOffsets(t, f.offsets.committed)
	endBefore := f.requireOffsets(t, f.offsets.end)

	f.proxy.setFault(fault)
	defer f.proxy.setFault(esFaultNone)
	trace := f.write(t, traceIDByte)

	require.Eventually(t, func() bool {
		end, err := f.offsets.end()
		return err == nil && end.advancedPast(endBefore)
	}, time.Minute, time.Second, "the trace never reached Kafka")
	t.Logf("Trace is in Kafka; holding the fault for %v", outageHoldTime)
	time.Sleep(outageHoldTime)

	committedDuring := f.requireOffsets(t, f.offsets.committed)
	endDuring := f.requireOffsets(t, f.offsets.end)
	t.Logf("Kafka offsets while the fault holds: committed=%v end=%v", committedDuring, endDuring)
	assert.Equal(t, committedBefore, committedDuring, "no partition's committed offset may advance while the write fails")
	duringOutage(t, trace)

	f.proxy.setFault(esFaultNone)
	t.Log("Fault lifted; waiting for recovery")
	// The offset catching up means the retried write succeeded, so every document
	// the recovery produced is in place before the count below.
	f.requireOffsetCaughtUp(t)
	f.requireStoredOnce(t, trace)
}

// outageSpanCount is the size of each trace the subtests write. It stays within
// the trace writer's MaxChunkSize so a trace is one OTLP request and, with the
// collector's batch processor, one Kafka record; a trace split across records
// would leave the later record unconsumed while the first one is retried.
var outageSpanCount = MaxChunkSize

// write sends a trace into the collector and returns it.
func (f *faultInjectionSteps) write(t *testing.T, traceIDByte byte) ptrace.Traces {
	trace := buildFaultInjectionTrace(traceIDByte, outageSpanCount)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	require.NoError(t, f.collector.TraceWriter.WriteTraces(ctx, trace))
	return trace
}

// requireOffsets reads offsets with one of the kafkaOffsets readers and fails the
// test on error. It is for reads outside a polling loop.
func (*faultInjectionSteps) requireOffsets(t *testing.T, read func() (partitionOffsets, error)) partitionOffsets {
	offsets, err := read()
	require.NoError(t, err)
	return offsets
}

// storedSpanCount returns how many span documents of the trace Elasticsearch holds
// right now, failing the test if Elasticsearch cannot be asked.
func (*faultInjectionSteps) storedSpanCount(t *testing.T, trace ptrace.Traces) int {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	count, err := countSpanDocs(ctx, traceID(trace))
	require.NoError(t, err)
	return count
}

// requireStoredOnce waits until every span of the trace is indexed and then checks
// that none is stored twice. A duplicated write shows up as an extra document with
// its own _id. Callers invoke it once the offset has caught up, so nothing is still
// in flight that could add a document after the count.
func (f *faultInjectionSteps) requireStoredOnce(t *testing.T, trace ptrace.Traces) {
	expected := trace.SpanCount()
	require.Eventually(t, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		count, err := countSpanDocs(ctx, traceID(trace))
		return err == nil && count >= expected
	}, 2*time.Minute, time.Second, "trace %s never became fully indexed", traceID(trace))
	assert.Equal(t, expected, f.storedSpanCount(t, trace), "trace %s must be stored exactly once", traceID(trace))
}

// requireOffsetCaughtUp waits until every partition's committed offset equals its
// end offset, and at least one partition holds a message.
func (f *faultInjectionSteps) requireOffsetCaughtUp(t *testing.T) {
	require.Eventually(t, func() bool {
		committed, err := f.offsets.committed()
		if err != nil {
			return false
		}
		end, err := f.offsets.end()
		if err != nil {
			return false
		}
		t.Logf("Kafka offsets: committed=%v end=%v", committed, end)
		return end.advancedPast(partitionOffsets{}) && maps.Equal(committed, end)
	}, 2*time.Minute, time.Second, "the committed offsets never caught up with the end offsets")
}

// traceID returns the trace ID shared by every span of a single-trace ptrace.Traces.
func traceID(trace ptrace.Traces) pcommon.TraceID {
	return trace.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).TraceID()
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
