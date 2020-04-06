// Copyright (c) 2020 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package reporter

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-lib/metrics/metricstest"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/cmd/agent/app/testutils"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
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
	} else {
		if assert.Equal(t, 1, logs.Len(), "expecting one log '%s'", msg) {
			field := logs.All()[0].ContextMap()["client-uuid"]
			assert.Equal(t, clientUUID, field, "client-uuid should be logged")
		}
	}
}

func testClientMetrics(fn func(tr *clientMetricsTest)) {
	testClientMetricsWithParams(ClientMetricsReporterParams{}, fn)
}

func testClientMetricsWithParams(params ClientMetricsReporterParams, fn func(tr *clientMetricsTest)) {
	r1 := testutils.NewInMemoryReporter()
	zapCore, logs := observer.New(zap.DebugLevel)
	mb := metricstest.NewFactory(time.Hour)

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
		assert.NoError(t, tr.r.EmitZipkinBatch([]*zipkincore.Span{{}}))
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

				err := tr.r.EmitBatch(batch)
				assert.NoError(t, err)
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
	params := ClientMetricsReporterParams{
		ExpireFrequency: 100 * time.Microsecond,
		ExpireTTL:       5 * time.Millisecond,
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

		err := tr.r.EmitBatch(batch)
		assert.NoError(t, err)
		assert.Len(t, tr.mr.Spans(), 1)

		// here we test that a connected-client gauge is updated to 1 by the auto-scheduled expire loop,
		// and then reset to 0 once the client entry expires.
		tests := []struct {
			expGauge int
			expLog   string
		}{
			{expGauge: 1, expLog: "new client"},
			{expGauge: 0, expLog: "freeing stats"},
		}
		start := time.Now()
		for i, test := range tests {
			t.Run(fmt.Sprintf("iter%d:gauge=%d,log=%s", i, test.expGauge, test.expLog), func(t *testing.T) {
				// Expire loop runs every 100us, and removes the client after 5ms.
				// We check for condition in each test for up to 5ms (10*500us).
				var gaugeValue int64 = -1
				for i := 0; i < 10; i++ {
					_, gauges := tr.mb.Snapshot()
					gaugeValue = gauges["client_stats.connected_clients"]
					if gaugeValue == int64(test.expGauge) {
						break
					}
					time.Sleep(1 * time.Millisecond)
				}
				assert.EqualValues(t, test.expGauge, gaugeValue)
				tr.assertLog(t, test.expLog, clientUUID)

				// sleep between tests long enough to exceed the 5ms TTL.
				if i == 0 {
					elapsed := time.Since(start)
					time.Sleep(5*time.Millisecond - elapsed)
				}
			})
		}
	})
}
