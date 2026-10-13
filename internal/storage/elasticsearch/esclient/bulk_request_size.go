// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"net/http"

	"github.com/elastic/go-elasticsearch/v9/esapi"

	"github.com/jaegertracing/jaeger/internal/metrics"
)

// requestBytesBuckets are the histogram boundaries, in bytes, for the size of one
// _bulk request body. They span from a near-empty interval flush up to the
// backend's default http.max_content_length (100 MB), with the finest resolution
// around the default 5 MB bulk_processing.max_bytes an operator tunes against.
var requestBytesBuckets = []float64{
	64 << 10,  // 64 KiB
	256 << 10, // 256 KiB
	1 << 20,   // 1 MiB
	2 << 20,   // 2 MiB
	4 << 20,   // 4 MiB
	8 << 20,   // 8 MiB
	16 << 20,  // 16 MiB
	32 << 20,  // 32 MiB
	64 << 20,  // 64 MiB
}

// newRequestBytesHistogram returns the bulk_index.request-bytes histogram, which
// both bulk writers record once per _bulk request with the size of the
// uncompressed request body, before transport compression. That is the size the
// flush thresholds (bulk_processing.max_bytes) operate on; with http_compression
// enabled the bytes on the wire are fewer. Its sum and count derive the flushed
// bytes per second and the requests per second, so there are no separate counters
// for either.
func newRequestBytesHistogram(factory metrics.Factory) metrics.Histogram {
	return factory.Namespace(metrics.NSOptions{Name: "bulk_index"}).Histogram(metrics.HistogramOptions{
		Name:    "request-bytes",
		Help:    "Size in bytes of the uncompressed body of each _bulk request, before transport compression",
		Buckets: requestBytesBuckets,
	})
}

// requestSizeTransport is the esapi.Transport that esutil.BulkIndexer sends its
// _bulk requests through. It records the size of each request's uncompressed body
// before delegating to the Client, whose elastictransport pool applies gzip when
// http_compression is enabled, so the value is the NDJSON size per flush rather
// than the bytes on the wire. esutil itself exposes no per-flush byte count: its
// BulkIndexerStats are cumulative across all workers and count documents, not bytes.
type requestSizeTransport struct {
	next         esapi.Transport
	requestBytes metrics.Histogram
}

func (t *requestSizeTransport) Perform(req *http.Request) (*http.Response, error) {
	// esutil builds the body from a bytes.Buffer, so http.NewRequest always sets
	// ContentLength to the uncompressed body size, and esutil never flushes an empty
	// buffer. The guard keeps an unknown length out of the histogram: net/http
	// reports it as 0 (or -1) with a body set.
	if req.ContentLength > 0 {
		t.requestBytes.Record(float64(req.ContentLength))
	}
	return t.next.Perform(req)
}
