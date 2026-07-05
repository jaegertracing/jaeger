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
// clients. It owns a multi-node connection pool with round-robin selection and
// failover, driven over a caller-supplied base RoundTripper. What that base
// applies is up to the caller: today the admin plane supplies TLS + Basic auth;
// once the full RoundTripper stack (`clientbuilder.GetHTTPRoundTripper`, with
// bearer/API-key/SigV4/custom_headers) is relocated into this package, both
// planes will share it. Payload-level clients compose over the pool.
type rawClient struct {
	pool *elastictransport.Client
}

// newPool constructs the underlying connection pool. It is a package var so tests
// can substitute a failing implementation to exercise the error path, rather than
// depending on elastic-transport's internal error conditions.
var newPool = elastictransport.NewClient

// newRawClient builds a rawClient that round-robins requests across servers,
// sending each through base. Node discovery (sniffing) is left disabled to match
// the current olivere behavior and avoid AWS/proxy misconfiguration.
func newRawClient(servers []string, base http.RoundTripper) (*rawClient, error) {
	urls := make([]*url.URL, 0, len(servers))
	for _, server := range servers {
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
	// Node discovery (sniffing) is left at its default of off. Retry is disabled
	// to preserve the current admin-client behavior; the data plane can opt into
	// the pool's read retry when it adopts rawClient in Stage B.
	pool, err := newPool(
		elastictransport.WithURLs(urls...),
		elastictransport.WithTransport(base),
		elastictransport.WithDisableRetry(),
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
