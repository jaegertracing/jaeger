// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

// This file holds the helpers behind TestKafkaStorage_SyncElasticsearch_DeadLetter
// and TestKafkaStorage_SyncElasticsearch_PoisonDrop in e2e_kafka_test.go: a poison
// trace Elasticsearch rejects deterministically, and an OTLP/HTTP server that stands
// in for the dead-letter sink and records what the pipeline sends it.

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.opentelemetry.io/collector/receiver/receivertest"
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

// deadLetterServer is an OTLP/HTTP traces endpoint, the stock otlp receiver in
// front of a consumertest sink, that records every span the dead-letter pipeline
// exports to it. While refusing is set it rejects every export with a retryable
// error instead, standing in for a dead-letter sink outage.
type deadLetterServer struct {
	endpoint string
	sink     *consumertest.TracesSink
	refusing atomic.Bool
	refusals atomic.Int32
}

// ConsumeTraces is the receiver's next consumer: the sink, or a refusal.
func (d *deadLetterServer) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	if d.refusing.Load() {
		d.refusals.Add(1)
		return errors.New("dead-letter sink is refusing exports")
	}
	return d.sink.ConsumeTraces(ctx, td)
}

func (*deadLetterServer) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

// refuse turns the outage on or off.
func (d *deadLetterServer) refuse(on bool) { d.refusing.Store(on) }

// refused returns how many exports were rejected so far.
func (d *deadLetterServer) refused() int { return int(d.refusals.Load()) }

func newDeadLetterServer(t *testing.T) *deadLetterServer {
	// Reserving the port by listening and closing is best effort: another process
	// could take it before the receiver binds, which surfaces as a Start error.
	ctx := context.Background()
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	d := &deadLetterServer{endpoint: ln.Addr().String(), sink: new(consumertest.TracesSink)}
	require.NoError(t, ln.Close())

	factory := otlpreceiver.NewFactory()
	cfg := factory.CreateDefaultConfig().(*otlpreceiver.Config)
	cfg.Protocols.GRPC = configoptional.None[configgrpc.ServerConfig]()
	// The HTTP protocol is off until materialized; enable it with its default URL
	// paths on the reserved port.
	cfg.Protocols.HTTP.GetOrInsertDefault().ServerConfig.NetAddr.Endpoint = d.endpoint

	receiver, err := factory.CreateTraces(ctx, receivertest.NewNopSettings(factory.Type()), cfg, d)
	require.NoError(t, err)
	require.NoError(t, receiver.Start(ctx, componenttest.NewNopHost()))
	t.Cleanup(func() { require.NoError(t, receiver.Shutdown(ctx)) })
	return d
}

// tracesURL is the endpoint for an otlphttp exporter's traces_endpoint.
func (d *deadLetterServer) tracesURL() string { return "http://" + d.endpoint + "/v1/traces" }

// received returns every span received so far.
func (d *deadLetterServer) received() []ptrace.Span {
	var out []ptrace.Span
	for _, td := range d.sink.AllTraces() {
		for _, rs := range td.ResourceSpans().All() {
			for _, ss := range rs.ScopeSpans().All() {
				for _, span := range ss.Spans().All() {
					out = append(out, span)
				}
			}
		}
	}
	return out
}

// writePoison sends a trace whose first span is a poison pill into the collector,
// waits for it to land in Kafka, and returns the trace and the poison span's id.
// Waiting for the record matters because requireOffsetCaughtUp would otherwise be
// satisfied by the offsets as they stood before the write. The offset reads need
// the topic to exist, and Kafka creates it on the first record, so a test's first
// step has to write an ordinary trace before this one.
func (f *faultInjectionSteps) writePoison(t *testing.T, traceIDByte byte) (ptrace.Traces, pcommon.SpanID) {
	logEndBefore := requireOffsets(t, f.offsets.logEnd)
	trace, poisonSpanID := buildPoisonTrace(traceIDByte, MaxChunkSize)
	f.send(t, trace)
	f.requireInKafka(t, logEndBefore, "the poison trace never reached Kafka")
	return trace, poisonSpanID
}

// requirePoisonStoredAround waits until the offset has caught up past the poison
// trace's record and then checks that every span of the trace except the poison one
// is stored exactly once: the batch completed around the poison pill instead of
// stalling on it.
func (f *faultInjectionSteps) requirePoisonStoredAround(t *testing.T, trace ptrace.Traces) {
	f.requireOffsetCaughtUp(t)
	f.requireStoredCount(t, trace, trace.SpanCount()-1, "trace %s must be stored once without its poison span")
}
