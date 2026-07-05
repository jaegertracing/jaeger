// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/elastic/elastic-transport-go/v8/elastictransport"
)

// rawClient is the shared low-level transport beneath the Elasticsearch/OpenSearch
// clients. It owns the horizontal transport concerns: a multi-node connection pool
// with round-robin selection and failover, driven over a caller-supplied
// RoundTripper stack that carries TLS, auth (basic/API-key/bearer/SigV4), and
// custom headers. Payload-level clients (the admin structs today, the data plane
// once olivere is replaced) compose over it, so there is a single transport and a
// single place that owns auth, headers, and node selection.
type rawClient struct {
	pool *elastictransport.Client
}

// newRawClient builds a rawClient that round-robins requests across servers,
// sending each through base — the RoundTripper stack that applies TLS, auth, and
// custom headers. Node discovery (sniffing) is left disabled to match the current
// olivere behavior and avoid AWS/proxy misconfiguration; the pool retries
// idempotent requests on 502/503/504 by default.
func newRawClient(servers []string, base http.RoundTripper) (*rawClient, error) {
	urls := make([]*url.URL, 0, len(servers))
	for _, server := range servers {
		u, err := url.Parse(server)
		if err != nil {
			return nil, fmt.Errorf("invalid server URL %q: %w", server, err)
		}
		urls = append(urls, u)
	}
	// Node discovery (sniffing) is left at its default of off.
	pool, err := elastictransport.NewClient(
		elastictransport.WithURLs(urls...),
		elastictransport.WithTransport(base),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build transport pool: %w", err)
	}
	return &rawClient{pool: pool}, nil
}

// perform sends req through the pool. req carries a relative path (e.g.
// "/_cluster/health"); the pool selects a node and fills in its scheme and host.
func (r *rawClient) perform(req *http.Request) (*http.Response, error) {
	return r.pool.Perform(req)
}
