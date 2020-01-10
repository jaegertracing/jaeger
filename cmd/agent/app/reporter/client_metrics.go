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
	"math"
	"sync"
	"time"

	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

const (
	// If client-reported counters wrap over MaxInt64, we can have old > new.
	// We will "detect" the wrapping by checking that old is within the tolerance
	// from MaxInt64 and new is within the tolerance from 0.
	wrappedCounterTolerance = 10000000
)

// clientMetrics are maintained only for data submitted in Jaeger Thrift format.
type clientMetrics struct {
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
	lock                  sync.Mutex
	lastUpdated           time.Time
	batchSeqNo            int64
	fullQueueDroppedSpans int64
	tooLargeDroppedSpans  int64
	failedToEmitSpans     int64
}

// ClientMetricsReporter is a decorator that emits data loss metrics on behalf of clients.
type ClientMetricsReporter struct {
	wrapped       Reporter
	logger        *zap.Logger
	clientMetrics *clientMetrics
	shutdown      chan struct{}

	// map from client-uuid to *lastReceivedClientStats
	lastReceivedClientStats sync.Map
}

// WrapWithClientMetrics creates ClientMetricsReporter.
func WrapWithClientMetrics(reporter Reporter, logger *zap.Logger, mFactory metrics.Factory) *ClientMetricsReporter {
	cm := new(clientMetrics)
	metrics.MustInit(cm, mFactory.Namespace(metrics.NSOptions{Name: "client_stats"}), nil)
	r := &ClientMetricsReporter{
		wrapped:       reporter,
		logger:        logger,
		clientMetrics: cm,
	}
	go r.expireClientMetrics()
	return r
}

// EmitZipkinBatch delegates to underlying Reporter.
func (r *ClientMetricsReporter) EmitZipkinBatch(spans []*zipkincore.Span) error {
	return r.wrapped.EmitZipkinBatch(spans)
}

// EmitBatch processes client data loss metrics and delegates to the underlying reporter.
func (r *ClientMetricsReporter) EmitBatch(batch *jaeger.Batch) error {
	r.updateClientMetrics(batch)
	return r.wrapped.EmitBatch(batch)
}

// Close stops background gc goroutine for client stats map.
func (r *ClientMetricsReporter) Close() {
	close(r.shutdown)
}

func (r *ClientMetricsReporter) expireClientMetrics() {
	const (
		frequency = 15 * time.Minute
		ttl       = time.Hour
	)
	ticker := time.NewTicker(frequency)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			t := time.Now()
			var size int64
			r.lastReceivedClientStats.Range(func(k, v interface{}) bool {
				stats := v.(*lastReceivedClientStats)
				stats.lock.Lock()
				defer stats.lock.Unlock()

				if !stats.lastUpdated.IsZero() && t.Sub(stats.lastUpdated) > ttl {
					r.lastReceivedClientStats.Delete(k)
					r.logger.Debug("have not heard from a client for a while, freeing stats",
						zap.Any("client-uuid", k),
						zap.Time("last-message", stats.lastUpdated),
					)
				}
				size += 1
				return true // keep running through all values in the map
			})
			r.clientMetrics.ConnectedClients.Update(size)
		case <-r.shutdown:
			return
		}
	}
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
			r.logger.Debug("received batch from a new client, starting to keep stats",
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

	if s.batchSeqNo >= batchSeqNo && !wrapped(s.batchSeqNo, batchSeqNo) {
		// ignore out of order batches, the metrics will be updated later
		return
	}

	metrics.BatchesSent.Inc(delta(s.batchSeqNo, batchSeqNo))

	if stats != nil {
		metrics.FailedToEmitSpans.Inc(delta(s.failedToEmitSpans, stats.FailedToEmitSpans))
		metrics.FailedToEmitSpans.Inc(delta(s.tooLargeDroppedSpans, stats.TooLargeDroppedSpans))
		metrics.FailedToEmitSpans.Inc(delta(s.fullQueueDroppedSpans, stats.FullQueueDroppedSpans))

		s.failedToEmitSpans = stats.FailedToEmitSpans
		s.tooLargeDroppedSpans = stats.TooLargeDroppedSpans
		s.fullQueueDroppedSpans = stats.FullQueueDroppedSpans
	}

	s.lastUpdated = time.Now()
	s.batchSeqNo = batchSeqNo
}

func wrapped(old int64, new int64) bool {
	return (old > math.MaxInt64-wrappedCounterTolerance) && (new < wrappedCounterTolerance)
}

func delta(old int64, new int64) int64 {
	if !wrapped(old, new) {
		return new - old
	}
	return new + (math.MaxInt64 - old)
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
