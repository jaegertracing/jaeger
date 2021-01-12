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
	"context"
	"sync"
	"time"

	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/atomic"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

const (
	defaultExpireFrequency = 15 * time.Minute
	defaultExpireTTL       = time.Hour
)

// clientMetrics are maintained only for data submitted in Jaeger Thrift format.
type clientMetrics struct {
	BatchesReceived  metrics.Counter `metric:"batches_received" help:"Total count of batches received from conforming clients"`
	BatchesSent      metrics.Counter `metric:"batches_sent" help:"Total count of batches sent by clients"`
	ConnectedClients metrics.Gauge   `metric:"connected_clients" help:"Total count of unique clients sending data to the agent"`

	// NB: The following three metrics all have the same name, but different "cause" tags.
	//     Only the first one is given a "help" struct tag, because Prometheus client combines
	//     them into one help entry in the /metrics endpoint, e.g.
	//
	//       # HELP jaeger_agent_client_stats_spans_dropped_total Total count of spans dropped by clients
	//       # TYPE jaeger_agent_client_stats_spans_dropped_total counter
	//       jaeger_agent_client_stats_spans_dropped_total{cause="full-queue"} 0
	//       jaeger_agent_client_stats_spans_dropped_total{cause="send-failure"} 0
	//       jaeger_agent_client_stats_spans_dropped_total{cause="too-large"} 0

	// Total count of spans dropped by clients because their internal queue were full.
	FullQueueDroppedSpans metrics.Counter `metric:"spans_dropped" tags:"cause=full-queue" help:"Total count of spans dropped by clients"`

	// Total count of spans dropped by clients because they were larger than max packet size.
	TooLargeDroppedSpans metrics.Counter `metric:"spans_dropped" tags:"cause=too-large"`

	// Total count of spans dropped by clients because they failed Thrift encoding or submission.
	FailedToEmitSpans metrics.Counter `metric:"spans_dropped" tags:"cause=send-failure"`
}

type lastReceivedClientStats struct {
	lock        sync.Mutex
	lastUpdated time.Time

	// Thrift stats are reported as signed i64, so keep the type to avoid multiple conversions back and forth.
	batchSeqNo            int64
	fullQueueDroppedSpans int64
	tooLargeDroppedSpans  int64
	failedToEmitSpans     int64
}

// ClientMetricsReporter is a decorator that emits data loss metrics on behalf of clients.
// The clients must send a Process.Tag `client-uuid` with a unique string for each client instance.
type ClientMetricsReporter struct {
	params        ClientMetricsReporterParams
	clientMetrics *clientMetrics
	shutdown      chan struct{}
	closed        *atomic.Bool

	// map from client-uuid to *lastReceivedClientStats
	lastReceivedClientStats sync.Map
}

// ClientMetricsReporterParams is used as input to WrapWithClientMetrics.
type ClientMetricsReporterParams struct {
	Reporter        Reporter        // required
	Logger          *zap.Logger     // required
	MetricsFactory  metrics.Factory // required
	ExpireFrequency time.Duration
	ExpireTTL       time.Duration
}

// WrapWithClientMetrics creates ClientMetricsReporter.
func WrapWithClientMetrics(params ClientMetricsReporterParams) *ClientMetricsReporter {
	if params.ExpireFrequency == 0 {
		params.ExpireFrequency = defaultExpireFrequency
	}
	if params.ExpireTTL == 0 {
		params.ExpireTTL = defaultExpireTTL
	}
	cm := new(clientMetrics)
	metrics.MustInit(cm, params.MetricsFactory.Namespace(metrics.NSOptions{Name: "client_stats"}), nil)
	r := &ClientMetricsReporter{
		params:        params,
		clientMetrics: cm,
		shutdown:      make(chan struct{}),
		closed:        atomic.NewBool(false),
	}
	go r.expireClientMetricsLoop()
	return r
}

// EmitZipkinBatch delegates to underlying Reporter.
func (r *ClientMetricsReporter) EmitZipkinBatch(ctx context.Context, spans []*zipkincore.Span) error {
	return r.params.Reporter.EmitZipkinBatch(ctx, spans)
}

// EmitBatch processes client data loss metrics and delegates to the underlying reporter.
func (r *ClientMetricsReporter) EmitBatch(ctx context.Context, batch *jaeger.Batch) error {
	r.updateClientMetrics(batch)
	return r.params.Reporter.EmitBatch(ctx, batch)
}

// Close stops background gc goroutine for client stats map.
func (r *ClientMetricsReporter) Close() error {
	if r.closed.CAS(false, true) {
		close(r.shutdown)
	}
	return nil
}

func (r *ClientMetricsReporter) expireClientMetricsLoop() {
	ticker := time.NewTicker(r.params.ExpireFrequency)
	defer ticker.Stop()
	for {
		select {
		case now := <-ticker.C:
			r.expireClientMetrics(now)
		case <-r.shutdown:
			return
		}
	}
}

func (r *ClientMetricsReporter) expireClientMetrics(t time.Time) {
	var size int64
	r.lastReceivedClientStats.Range(func(k, v interface{}) bool {
		stats := v.(*lastReceivedClientStats)
		stats.lock.Lock()
		defer stats.lock.Unlock()

		if !stats.lastUpdated.IsZero() && t.Sub(stats.lastUpdated) > r.params.ExpireTTL {
			r.lastReceivedClientStats.Delete(k)
			r.params.Logger.Debug("have not heard from a client for a while, freeing stats",
				zap.Any("client-uuid", k),
				zap.Time("last-message", stats.lastUpdated),
			)
		}
		size++
		return true // keep running through all values in the map
	})
	r.clientMetrics.ConnectedClients.Update(size)
}

func (r *ClientMetricsReporter) updateClientMetrics(batch *jaeger.Batch) {
	clientUUID := clientUUID(batch)
	if clientUUID == "" {
		return
	}
	if batch.SeqNo == nil {
		return
	}
	entry, found := r.lastReceivedClientStats.Load(clientUUID)
	if !found {
		ent, loaded := r.lastReceivedClientStats.LoadOrStore(clientUUID, &lastReceivedClientStats{})
		if !loaded {
			r.params.Logger.Debug("received batch from a new client, starting to keep stats",
				zap.String("client-uuid", clientUUID),
			)
		}
		entry = ent
	}
	clientStats := entry.(*lastReceivedClientStats)
	clientStats.update(*batch.SeqNo, batch.Stats, r.clientMetrics)
}

func (s *lastReceivedClientStats) update(
	batchSeqNo int64,
	stats *jaeger.ClientStats,
	metrics *clientMetrics,
) {
	s.lock.Lock()
	defer s.lock.Unlock()

	metrics.BatchesReceived.Inc(1)

	if s.batchSeqNo >= batchSeqNo {
		// Ignore out of order batches. Once we receive a batch with a larger-than-seen number,
		// it will contain new cumulative counts, which we will use to update the metrics.
		// That makes the metrics slightly off in time, but accurate in aggregate.
		return
	}
	// do not update counters on the first batch, because it may cause a huge spike in totals
	// if the client has been running for a while already, but the agent just started.
	if s.batchSeqNo > 0 {
		metrics.BatchesSent.Inc(batchSeqNo - s.batchSeqNo)
		if stats != nil {
			metrics.FailedToEmitSpans.Inc(stats.FailedToEmitSpans - s.failedToEmitSpans)
			metrics.TooLargeDroppedSpans.Inc(stats.TooLargeDroppedSpans - s.tooLargeDroppedSpans)
			metrics.FullQueueDroppedSpans.Inc(stats.FullQueueDroppedSpans - s.fullQueueDroppedSpans)
		}
	}

	s.lastUpdated = time.Now()
	s.batchSeqNo = batchSeqNo
	if stats != nil {
		s.failedToEmitSpans = stats.FailedToEmitSpans
		s.tooLargeDroppedSpans = stats.TooLargeDroppedSpans
		s.fullQueueDroppedSpans = stats.FullQueueDroppedSpans
	}
}

func clientUUID(batch *jaeger.Batch) string {
	if batch.Process == nil {
		return ""
	}
	for _, tag := range batch.Process.Tags {
		if tag.Key != "client-uuid" {
			continue
		}
		if tag.VStr == nil {
			return ""
		}
		return *tag.VStr
	}
	return ""
}
