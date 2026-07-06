// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/elastic/go-elasticsearch/v9/esutil"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/spanstoremetrics"
)

// BulkItem is a single document to write via the bulk API.
type BulkItem struct {
	Index  string         // target index, alias, or data stream
	ID     string         // optional document _id (empty ⇒ server-generated)
	OpType es.WriteOpType // "index" (default) or "create"
	Body   any            // JSON-serializable source document
}

// BulkIndexerConfig bounds a BulkIndexer's buffering and concurrency.
type BulkIndexerConfig struct {
	FlushBytes    int           // flush once a worker's buffer reaches this many bytes
	FlushInterval time.Duration // flush a partial buffer after this long
	Workers       int           // number of concurrent flush workers
}

// BulkIndexer writes documents to _bulk over the shared transport. It wraps the
// official go-elasticsearch esutil.BulkIndexer — a bounded, worker-pooled indexer
// that flushes on a byte threshold or interval (fixing #2192) — driven by our own
// transport pool (via the esapi.Transport interface), so no product-checked
// go-elasticsearch client is involved.
type BulkIndexer struct {
	bi      esutil.BulkIndexer
	metrics *spanstoremetrics.WriteMetrics
	logger  *zap.Logger
}

var _ BulkWriter = (*BulkIndexer)(nil)

type flushStartKey struct{}

// NewBulkIndexer returns a running BulkIndexer. The caller owns its lifecycle and
// must call Close to flush buffered documents and stop the workers.
func NewBulkIndexer(client Client, cfg BulkIndexerConfig, metricsFactory metrics.Factory, logger *zap.Logger) (*BulkIndexer, error) {
	b := &BulkIndexer{
		metrics: spanstoremetrics.NewWriter(metricsFactory, "bulk_index"),
		logger:  logger,
	}
	// Default to a single worker (matching the historical olivere BulkProcessor
	// default); esutil would otherwise fan out to NumCPU workers. This also keeps
	// concurrent requests off the shared, not-yet-thread-safe auth token provider.
	workers := cfg.Workers
	if workers <= 0 {
		workers = 1
	}
	bi, err := esutil.NewBulkIndexer(esutil.BulkIndexerConfig{
		Client:        client.transport.pool, // *elastictransport.Client satisfies esapi.Transport
		NumWorkers:    workers,
		FlushBytes:    cfg.FlushBytes,
		FlushInterval: cfg.FlushInterval,
		OnError: func(_ context.Context, err error) {
			logger.Error("bulk indexer error", zap.Error(err))
		},
		OnFlushStart: func(ctx context.Context) context.Context {
			return context.WithValue(ctx, flushStartKey{}, time.Now())
		},
		OnFlushEnd: func(ctx context.Context) {
			if start, ok := ctx.Value(flushStartKey{}).(time.Time); ok {
				b.metrics.LatencyOk.Record(time.Since(start))
			}
		},
	})
	if err != nil {
		return nil, err
	}
	b.bi = bi
	return b, nil
}

// Add encodes and enqueues a document. Encoding or enqueue failures are logged
// and counted; the bulk API's own semantics (buffering, flush, per-item results)
// are handled by esutil.
func (b *BulkIndexer) Add(item BulkItem) {
	body, err := json.Marshal(item.Body)
	if err != nil {
		b.logger.Error("failed to encode bulk document", zap.String("index", item.Index), zap.Error(err))
		return
	}
	action := string(item.OpType)
	if action == "" {
		action = string(es.WriteOpIndex)
	}
	err = b.bi.Add(context.Background(), esutil.BulkIndexerItem{
		Action:     action,
		Index:      item.Index,
		DocumentID: item.ID,
		Body:       bytes.NewReader(body),
		OnSuccess: func(context.Context, esutil.BulkIndexerItem, esutil.BulkIndexerResponseItem) {
			b.metrics.Attempts.Inc(1)
			b.metrics.Inserts.Inc(1)
		},
		OnFailure: func(_ context.Context, _ esutil.BulkIndexerItem, resp esutil.BulkIndexerResponseItem, itemErr error) {
			b.metrics.Attempts.Inc(1)
			b.metrics.Errors.Inc(1)
			b.metrics.LatencyErr.Record(0)
			b.logger.Error("Elasticsearch part of bulk request failed",
				zap.String("index", item.Index), zap.Int("status", resp.Status), zap.Error(itemErr))
		},
	})
	if err != nil {
		b.logger.Error("failed to enqueue bulk document", zap.String("index", item.Index), zap.Error(err))
	}
}

// Close flushes buffered documents and stops the workers.
func (b *BulkIndexer) Close() error {
	return b.bi.Close(context.Background())
}
