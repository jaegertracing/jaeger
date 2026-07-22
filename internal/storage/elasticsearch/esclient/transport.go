// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/elastic/elastic-transport-go/v8/elastictransport"
	"go.uber.org/zap"
)

// rawClient is the shared low-level transport beneath the Elasticsearch/OpenSearch
// clients. It owns a multi-node connection pool with round-robin selection and
// failover, driven over a caller-supplied base RoundTripper — the full stack from
// GetHTTPRoundTripper (TLS + basic/bearer/API-key/SigV4/custom_headers). Every
// data-plane and admin-plane client composes over the pool.
type rawClient struct {
	pool *elastictransport.Client
	// base is the RoundTripper stack the pool sends through; kept so Close can
	// release its idle connections.
	base http.RoundTripper
}

// close releases idle connections held by the base transport, if it supports it.
func (r *rawClient) close() {
	if c, ok := r.base.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}

// newPool constructs the underlying connection pool. It is a package var so tests
// can substitute a failing implementation to exercise the error path, rather than
// depending on elastic-transport's internal error conditions.
var newPool = elastictransport.NewClient

// rawClientOptions carries the connection settings newRawClient reads off the
// Elasticsearch configuration, keeping the constructor's signature readable.
type rawClientOptions struct {
	servers []string
	// compressRequestBody gzips outgoing request bodies.
	compressRequestBody bool
	// discoverNodes enables one-shot node discovery (sniffing) at startup.
	discoverNodes bool
	// logLevel selects the client log-level (debug/info/error); empty disables
	// client logging. logger is the destination for those logs.
	logLevel string
	logger   *zap.Logger
}

// newRawClient builds a rawClient that round-robins requests across servers,
// sending each through base.
func newRawClient(ctx context.Context, base http.RoundTripper, opts rawClientOptions) (*rawClient, error) {
	if len(opts.servers) == 0 {
		return nil, errors.New("no servers specified")
	}
	urls := make([]*url.URL, 0, len(opts.servers))
	for _, server := range opts.servers {
		u, err := url.Parse(server)
		if err != nil {
			return nil, fmt.Errorf("invalid server URL %q: %w", server, err)
		}
		// url.Parse accepts host:port or bare hosts as scheme/path-only URLs; the
		// pool needs a scheme and host, so reject those up front with a clear error.
		if u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("server URL %q must include a scheme and host, e.g. http://host:9200", server)
		}
		urls = append(urls, u)
	}
	// Retry is disabled to preserve the current admin-client behavior; the data
	// plane can opt into the pool's read retry when it adopts rawClient in Stage B.
	transportOpts := []elastictransport.Option{
		elastictransport.WithURLs(urls...),
		elastictransport.WithTransport(base),
		elastictransport.WithDisableRetry(),
	}
	if opts.compressRequestBody {
		// Defaults to gzip.DefaultCompression, matching the level the olivere
		// client used before it was retired. The pool gzips the body and sets
		// Content-Encoding before handing the request to base, so the auth stack
		// below (notably the SigV4 signer) signs the compressed payload that
		// actually goes on the wire.
		transportOpts = append(transportOpts, elastictransport.WithCompression())
	}
	if opts.logLevel != "" && opts.logger != nil {
		transportOpts = append(transportOpts, elastictransport.WithLogger(newZapLogger(opts.logLevel, opts.logger)))
	}
	pool, err := newPool(transportOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to build transport pool: %w", err)
	}
	if opts.discoverNodes {
		// Node discovery (sniffing): query one seed node once at startup and add
		// the cluster's other nodes to the pool. This is a one-shot call — no
		// background goroutine is scheduled (that would require a discovery
		// interval), so close() still has nothing to stop. Left off by default
		// because a cluster that publishes addresses the client cannot reach (a
		// common AWS/proxy setup) would break the pool.
		if err := pool.DiscoverNodesContext(ctx); err != nil {
			return nil, fmt.Errorf("failed to discover Elasticsearch nodes (sniffing): %w", err)
		}
	}
	return &rawClient{pool: pool, base: base}, nil
}

// perform sends req through the pool. req carries a relative path (e.g.
// "/_cluster/health"); the pool selects a node and fills in its scheme and host.
func (r *rawClient) perform(req *http.Request) (*http.Response, error) {
	return r.pool.Perform(req)
}
