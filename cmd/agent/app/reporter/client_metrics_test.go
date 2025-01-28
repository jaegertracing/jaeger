// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package reporter

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger-idl/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger-idl/thrift-gen/zipkincore"
	"github.com/jaegertracing/jaeger/cmd/agent/app/testutils"
	"github.com/jaegertracing/jaeger/internal/metricstest"
)

type clientMetricsTest struct {
	mr   *testutils.InMemoryReporter
	r    *ClientMetricsReporter
	logs *observer.ObservedLogs
	mb   *metricstest.Factory
}

func (tr *clientMetricsTest) assertLog(t *testing.T, msg, clientUUID string) {
	logs := tr.logs.FilterMessageSnippet(msg)
	if clientUUID == "" {
		assert.Equal(t, 0, logs.Len(), "not expecting log '%s", msg)
	} else if assert.Equal(t, 1, logs.Len(), "expecting one log '%s'", msg) {
		field := logs.All()[0].ContextMap()["client-uuid"]
		assert.Equal(t, clientUUID, field, "client-uuid should be logged")
	}
}

func testClientMetrics(fn func(tr *clientMetricsTest)) {
	testClientMetricsWithParams(ClientMetricsReporterParams{}, fn)
}

func testClientMetricsWithParams(params ClientMetricsReporterParams, fn func(tr *clientMetricsTest)) {
	r1 := testutils.NewInMemoryReporter()
	zapCore, logs := observer.New(zap.DebugLevel)
	mb := metricstest.NewFactory(time.Hour)
	defer mb.Stop()

	params.Reporter = r1
	params.Logger = zap.New(zapCore)
	params.MetricsFactory = mb

	r := WrapWithClientMetrics(params)
	defer r.Close()

	tr := &clientMetricsTest{
		mr:   r1,
		r:    r,
		logs: logs,
		mb:   mb,
	}
	fn(tr)
}

func TestClientMetricsReporter_Zipkin(t *testing.T) {
	testClientMetrics(func(tr *clientMetricsTest) {
		require.NoError(t, tr.r.EmitZipkinBatch(context.Background(), []*zipkincore.Span{{}}))
		assert.Len(t, tr.mr.ZipkinSpans(), 1)
	})
}

func TestClientMetricsReporter_Jaeger(t *testing.T) {
	testClientMetrics(func(tr *clientMetricsTest) {
		blank := ""
		clientUUID := "foobar"
		nPtr := func(v int64) *int64 { return &v }
		const prefix = "client_stats."
		tag := func(name, value string) map[string]string { return map[string]string{name: value} }

		tests := []struct {
			clientUUID  *string
			seqNo       *int64
			stats       *jaeger.ClientStats
			runExpire   bool // invoke expireClientMetrics to update the gauge
			expLog      string
			expCounters []metricstest.ExpectedMetric
			expGauges   []metricstest.ExpectedMetric
		}{
			{},
			{clientUUID: &blank},
			{clientUUID: &clientUUID},
			{
				clientUUID: &clientUUID,
				seqNo:      nPtr(100),
				expLog:     clientUUID,
				stats: &jaeger.ClientStats{
					FullQueueDroppedSpans: 10,
					TooLargeDroppedSpans:  10,
					FailedToEmitSpans:     10,
				},
				runExpire: true,
				// first batch cannot increment counters, only capture the baseline
				expCounters: []metricstest.ExpectedMetric{
					{Name: prefix + "batches_received", Value: 1},
					{Name: prefix + "batches_sent", Value: 0},
					{Name: prefix + "spans_dropped", Tags: tag("cause", "full-queue"), Value: 0},
					{Name: prefix + "spans_dropped", Tags: tag("cause", "too-large"), Value: 0},
					{Name: prefix + "spans_dropped", Tags: tag("cause", "send-failure"), Value: 0},
				},
				expGauges: []metricstest.ExpectedMetric{
					{Name: prefix + "connected_clients", Value: 1},
				},
			},
			{
				clientUUID: &clientUUID,
				seqNo:      nPtr(105),
				stats: &jaeger.ClientStats{
					FullQueueDroppedSpans: 15,
					TooLargeDroppedSpans:  15,
					FailedToEmitSpans:     15,
				},
				expCounters: []metricstest.ExpectedMetric{
					{Name: prefix + "batches_received", Value: 2},
					{Name: prefix + "batches_sent", Value: 5},
					{Name: prefix + "spans_dropped", Tags: tag("cause", "full-queue"), Value: 5},
					{Name: prefix + "spans_dropped", Tags: tag("cause", "too-large"), Value: 5},
					{Name: prefix + "spans_dropped", Tags: tag("cause", "send-failure"), Value: 5},
				},
			},
			{
				clientUUID: &clientUUID,
				seqNo:      nPtr(90), // out of order batch will be ignored
				expCounters: []metricstest.ExpectedMetric{
					{Name: prefix + "batches_received", Value: 3},
					{Name: prefix + "batches_sent", Value: 5}, // unchanged!
				},
			},
			{
				clientUUID: &clientUUID,
				seqNo:      nPtr(110),
				// use different stats values to test the correct assignments
				stats: &jaeger.ClientStats{
					FullQueueDroppedSpans: 17,
					TooLargeDroppedSpans:  18,
					FailedToEmitSpans:     19,
				}, expCounters: []metricstest.ExpectedMetric{
					{Name: prefix + "batches_received", Value: 4},
					{Name: prefix + "batches_sent", Value: 10},
					{Name: prefix + "spans_dropped", Tags: tag("cause", "full-queue"), Value: 7},
					{Name: prefix + "spans_dropped", Tags: tag("cause", "too-large"), Value: 8},
					{Name: prefix + "spans_dropped", Tags: tag("cause", "send-failure"), Value: 9},
				},
			},
		}

		for i, test := range tests {
			t.Run(fmt.Sprintf("iter%d", i), func(t *testing.T) {
				tr.logs.TakeAll()

				batch := &jaeger.Batch{
					Spans: []*jaeger.Span{{}},
					Process: &jaeger.Process{
						ServiceName: "blah",
					},
					SeqNo: test.seqNo,
					Stats: test.stats,
				}
				if test.clientUUID != nil {
					batch.Process.Tags = []*jaeger.Tag{{Key: "client-uuid", VStr: test.clientUUID}}
				}

				err := tr.r.EmitBatch(context.Background(), batch)
				require.NoError(t, err)
				assert.Len(t, tr.mr.Spans(), i+1)

				tr.assertLog(t, "new client", test.expLog)

				if test.runExpire {
					tr.r.expireClientMetrics(time.Now())
				}

				if m := test.expCounters; len(m) > 0 {
					tr.mb.AssertCounterMetrics(t, m...)
				}
				if m := test.expGauges; len(m) > 0 {
					tr.mb.AssertGaugeMetrics(t, m...)
				}
			})
		}
	})
}

func TestClientMetricsReporter_ClientUUID(t *testing.T) {
	id := "my-client-id"
	tests := []struct {
		process    *jaeger.Process
		clientUUID string
	}{
		{process: nil, clientUUID: ""},
		{process: &jaeger.Process{}, clientUUID: ""},
		{process: &jaeger.Process{Tags: []*jaeger.Tag{}}, clientUUID: ""},
		{process: &jaeger.Process{Tags: []*jaeger.Tag{{Key: "blah"}}}, clientUUID: ""},
		{process: &jaeger.Process{Tags: []*jaeger.Tag{{Key: "client-uuid"}}}, clientUUID: ""},
		{process: &jaeger.Process{Tags: []*jaeger.Tag{{Key: "client-uuid", VStr: &id}}}, clientUUID: id},
	}
	for i, test := range tests {
		t.Run(fmt.Sprintf("iter%d", i), func(t *testing.T) {
			assert.Equal(t, test.clientUUID, clientUUID(&jaeger.Batch{Process: test.process}))
		})
	}
}

func TestClientMetricsReporter_Expire(t *testing.T) {
	const expireTTL = 50 * time.Millisecond
	params := ClientMetricsReporterParams{
		ExpireFrequency: 1 * time.Millisecond,
		ExpireTTL:       expireTTL,
	}
	testClientMetricsWithParams(params, func(tr *clientMetricsTest) {
		nPtr := func(v int64) *int64 { return &v }
		clientUUID := "blah"
		batch := &jaeger.Batch{
			Spans: []*jaeger.Span{{}},
			Process: &jaeger.Process{
				ServiceName: "blah",
				Tags:        []*jaeger.Tag{{Key: "client-uuid", VStr: &clientUUID}},
			},
			SeqNo: nPtr(1),
		}

		getGauge := func() int64 {
			_, gauges := tr.mb.Snapshot()
			return gauges["client_stats.connected_clients"]
		}

		t.Run("detect new client", func(t *testing.T) {
			assert.EqualValues(t, 0, getGauge(), "start with gauge=0")

			err := tr.r.EmitBatch(context.Background(), batch)
			require.NoError(t, err)
			assert.Len(t, tr.mr.Spans(), 1)

			// we want this test to pass asap, but need to account for possible CPU contention in the CI
			var gauge int64
			for i := 0; i < 1000; i++ {
				time.Sleep(1 * time.Millisecond)
				if gauge = getGauge(); gauge == 1 {
					t.Logf("gauge=1 detected on iteration %d", i)
					tr.assertLog(t, "new client", clientUUID)
					return
				}
			}
			require.EqualValues(t, 1, gauge)
		})

		t.Run("detect stale client", func(t *testing.T) {
			assert.EqualValues(t, 1, getGauge(), "start with gauge=1")

			time.Sleep(expireTTL)

			var gauge int64
			for i := 0; i < 1000; i++ {
				time.Sleep(1 * time.Millisecond)
				if gauge = getGauge(); gauge == 0 {
					t.Logf("gauge=0 detected on iteration %d", i)
					tr.assertLog(t, "freeing stats", clientUUID)
					return
				}
			}
			require.EqualValues(t, 0, gauge)
		})
	})
}
