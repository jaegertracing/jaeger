// Copyright (c) 2018 The Jaeger Authors.
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
	"sync"
	"time"

	"github.com/uber/jaeger-lib/metrics"

	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

const (
	jaegerBatches = "jaeger"
	zipkinBatches = "zipkin"
)

type batchMetrics struct {
	// Number of successful batch submissions to collector
	BatchesSubmitted metrics.Counter `metric:"batches.submitted"`

	// Number of failed batch submissions to collector
	BatchesFailures metrics.Counter `metric:"batches.failures"`

	// Number of spans in a batch submitted to collector
	BatchSize metrics.Gauge `metric:"batch_size"`

	// Number of successful span submissions to collector
	SpansSubmitted metrics.Counter `metric:"spans.submitted"`

	// Number of failed span submissions to collector
	SpansFailures metrics.Counter `metric:"spans.failures"`
}

// clientMetrics are maintained only for data submitted in Jaeger Thrift format.
type clientMetrics struct {
	Batches          metrics.Counter `metric:"batches_sent" help:"Total count of batches sent by clients"`
	ConnectedClients metrics.Gauge   `metric:"connected_clients" help:"Total count of unique clients sending data to the agent"`

	FullQueueDroppedSpans metrics.Counter `metric:"spans_dropped" tags:"cause=full-queue" help:"Total count of spans dropped by clients because their internal queue were full"`
	TooLargeDroppedSpans  metrics.Counter `metric:"spans_dropped" tags:"cause=too-large" help:"Total count of spans dropped by clients because they were larger than max packet size"`
	FailedToEmitSpans     metrics.Counter `metric:"spans_dropped" tags:"cause=send-failure" help:"Total count of spans dropped by clients because they failed Thrift encoding or submission"`
}

type lastReceivedClientStats struct {
	lock                  sync.Mutex
	lastUpdated           time.Time
	batchSeqNo            int64
	fullQueueDroppedSpans int64
	tooLargeDroppedSpans  int64
	failedToEmitSpans     int64
}

// MetricsReporter is reporter with metrics integration.
type MetricsReporter struct {
	wrapped Reporter

	// counters grouped by the type of data format (Jaeger or Zipkin).
	metrics map[string]batchMetrics

	clientMetrics *clientMetrics

	// map from client-uuid to *lastReceivedClientStats
	lastReceivedClientStats sync.Map

	shutdown chan struct{}
}

// WrapWithMetrics wraps Reporter and creates metrics for its invocations.
func WrapWithMetrics(reporter Reporter, mFactory metrics.Factory) *MetricsReporter {
	batchesMetrics := map[string]batchMetrics{}
	for _, s := range []string{zipkinBatches, jaegerBatches} {
		bm := batchMetrics{}
		metrics.MustInit(&bm,
			mFactory.Namespace(metrics.NSOptions{
				Name: "reporter", Tags: map[string]string{"format": s},
			}),
			nil)
		batchesMetrics[s] = bm
	}
	cm := new(clientMetrics)
	metrics.MustInit(cm, mFactory.Namespace(metrics.NSOptions{Name: "client_stats"}), nil)
	r := &MetricsReporter{
		wrapped:       reporter,
		metrics:       batchesMetrics,
		clientMetrics: cm,
	}
	go r.expireClientMetrics()
	return r
}

// EmitZipkinBatch emits batch to collector.
func (r *MetricsReporter) EmitZipkinBatch(spans []*zipkincore.Span) error {
	err := r.wrapped.EmitZipkinBatch(spans)
	updateMetrics(r.metrics[zipkinBatches], int64(len(spans)), err)
	return err
}

// EmitBatch emits batch to collector.
func (r *MetricsReporter) EmitBatch(batch *jaeger.Batch) error {
	r.updateClientMetrics(batch)
	size := int64(0)
	if batch != nil {
		size = int64(len(batch.GetSpans()))
	}
	err := r.wrapped.EmitBatch(batch)
	updateMetrics(r.metrics[jaegerBatches], size, err)
	return err
}

// Close stops background gc goroutine for client stats map.
func (r *MetricsReporter) Close() {
	close(r.shutdown)
}

func updateMetrics(m batchMetrics, size int64, err error) {
	if err != nil {
		m.BatchesFailures.Inc(1)
		m.SpansFailures.Inc(size)
	} else {
		m.BatchSize.Update(size)
		m.BatchesSubmitted.Inc(1)
		m.SpansSubmitted.Inc(size)
	}
}

func (r *MetricsReporter) expireClientMetrics() {
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

func (r *MetricsReporter) updateClientMetrics(batch *jaeger.Batch) {
	clientUUID := findClientUUID(batch)
	if clientUUID == "" {
		return
	}
	batchSeqNo := batch.SeqNo
	if batchSeqNo == nil {
		return
	}
	entry, found := r.lastReceivedClientStats.Load(clientUUID)
	if !found {
		entry, _ = r.lastReceivedClientStats.LoadOrStore(clientUUID, &lastReceivedClientStats{})
	}
	clientStats := entry.(*lastReceivedClientStats)
	clientStats.update(*batchSeqNo, batch.Stats, r.clientMetrics)
}

func (s *lastReceivedClientStats) update(
	batchSeqNo int64,
	stats *jaeger.ClientStats,
	metrics *clientMetrics,
) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if batchSeqNo <= s.batchSeqNo {
		// ignore out of order batches, the metrics will be updated later
		return
	}

	metrics.Batches.Inc(delta(s.batchSeqNo, batchSeqNo))

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

func delta(old int64, new int64) int64 {
	// TODO handle overflow
	return new - old
}

func findClientUUID(batch *jaeger.Batch) string {
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
