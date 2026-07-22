// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elastic/elastic-transport-go/v8/elastictransport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configtls"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
)

// headerStampRoundTripper stands in for the auth/header stack: it stamps a header
// on every request so a test can prove the base RoundTripper is applied.
type headerStampRoundTripper struct {
	base  http.RoundTripper
	key   string
	value string
}

func (rt headerStampRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set(rt.key, rt.value)
	return rt.base.RoundTrip(req)
}

func TestRawClientRoundRobinsAndAppliesBase(t *testing.T) {
	var hits1, hits2 atomic.Int32
	var sawHeader atomic.Bool
	handler := func(hits *atomic.Int32) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			if r.Header.Get("X-Jaeger-Test") == "applied" {
				sawHeader.Store(true)
			}
			w.WriteHeader(http.StatusOK)
		}
	}
	server1 := httptest.NewServer(handler(&hits1))
	defer server1.Close()
	server2 := httptest.NewServer(handler(&hits2))
	defer server2.Close()

	base := headerStampRoundTripper{base: http.DefaultTransport, key: "X-Jaeger-Test", value: "applied"}
	rc, err := newRawClient(t.Context(), base, rawClientOptions{servers: []string{server1.URL, server2.URL}})
	require.NoError(t, err)

	const requests = 6
	for range requests {
		// A relative path: the pool fills in the selected node's scheme and host.
		req, err := http.NewRequest(http.MethodGet, "/_cluster/health", http.NoBody)
		require.NoError(t, err)
		resp, err := rc.perform(req)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
	}

	assert.Positive(t, hits1.Load(), "first node received no requests")
	assert.Positive(t, hits2.Load(), "second node received no requests")
	assert.Equal(t, int32(requests), hits1.Load()+hits2.Load(), "every request reached exactly one node")
	assert.True(t, sawHeader.Load(), "the base RoundTripper stack was not applied to pooled requests")
}

// TestNewRawClientNoServers covers the empty-servers guard, which reproduces the
// error the retired legacy client raised so callers (the storage factory) still
// fail construction with the same message on a missing Servers list.
func TestNewRawClientNoServers(t *testing.T) {
	for name, servers := range map[string][]string{"nil": nil, "empty": {}} {
		t.Run(name, func(t *testing.T) {
			_, err := newRawClient(t.Context(), http.DefaultTransport, rawClientOptions{servers: servers})
			require.EqualError(t, err, "no servers specified")
		})
	}
}

func TestNewRawClientInvalidURL(t *testing.T) {
	tests := map[string]string{
		"unparseable":    "http://host:notaport",
		"missing scheme": "localhost:9200",
		"bare host":      "localhost",
	}
	for name, server := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := newRawClient(t.Context(), http.DefaultTransport, rawClientOptions{servers: []string{server}})
			require.Error(t, err)
		})
	}
}

// TestRawClientHonorsServerPathPrefix guards that a path prefix on the server URL
// (e.g. behind a reverse proxy) is preserved: the pool prepends the connection's
// path to the request's relative path.
func TestRawClientHonorsServerPathPrefix(t *testing.T) {
	var gotPath atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath.Store(r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	rc, err := newRawClient(t.Context(), http.DefaultTransport, rawClientOptions{servers: []string{server.URL + "/es"}})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodGet, "/_cluster/health", http.NoBody)
	require.NoError(t, err)
	resp, err := rc.perform(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	assert.Equal(t, "/es/_cluster/health", gotPath.Load())
}

// TestNewClientAppliesTLSConfig verifies that NewClient threads the tls.Config
// into the transport: an untrusted server cert fails, and InsecureSkipVerify
// makes it succeed.
func TestNewClientAppliesTLSConfig(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	strict, err := NewClient(t.Context(), &config.Configuration{
		Servers: []string{server.URL},
		Version: uint(es.ElasticV7), // pin so NewClient doesn't probe; test the request path
	}, zap.NewNop(), nil)
	require.NoError(t, err)
	_, err = strict.request(t.Context(), elasticRequest{method: http.MethodGet, endpoint: ""})
	require.Error(t, err, "an untrusted self-signed cert must fail TLS verification")

	insecure, err := NewClient(t.Context(), &config.Configuration{
		Servers: []string{server.URL},
		Version: uint(es.ElasticV7),
		TLS:     configtls.ClientConfig{Insecure: true},
	}, zap.NewNop(), nil)
	require.NoError(t, err)
	_, err = insecure.request(t.Context(), elasticRequest{method: http.MethodGet, endpoint: ""})
	require.NoError(t, err, "InsecureSkipVerify must be honored")
}

// TestNewClientHonorsHTTPCompression guards the wiring of the http_compression
// setting through to the transport pool. The option is on by default and gzipped
// every request body until the olivere client was retired (#8982), after which it
// was parsed but read by nothing, so bodies silently went out uncompressed (#9071).
func TestNewClientHonorsHTTPCompression(t *testing.T) {
	// An NDJSON _bulk body: the write path this regression actually degraded.
	payload := []byte(`{"index":{"_index":"jaeger-span-write"}}` + "\n" +
		`{"traceID":"1234567890abcdef","operationName":"test-operation"}` + "\n")
	tests := []struct {
		name         string
		compression  bool
		wantEncoding string
	}{
		{name: "compression enabled", compression: true, wantEncoding: "gzip"},
		{name: "compression disabled", compression: false, wantEncoding: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var gotEncoding, gotBody atomic.Value
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotEncoding.Store(r.Header.Get("Content-Encoding"))
				b, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				gotBody.Store(b)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			c, err := NewClient(t.Context(), &config.Configuration{
				Servers:         []string{server.URL},
				HTTPCompression: test.compression,
				Version:         uint(es.ElasticV7), // pin so NewClient doesn't probe
			}, zap.NewNop(), nil)
			require.NoError(t, err)

			_, err = c.request(t.Context(), elasticRequest{
				method:   http.MethodPost,
				endpoint: "_bulk",
				body:     payload,
			})
			require.NoError(t, err)

			assert.Equal(t, test.wantEncoding, gotEncoding.Load())
			body, ok := gotBody.Load().([]byte)
			require.True(t, ok)
			if !test.compression {
				assert.Equal(t, payload, body)
				return
			}
			// The server must receive real gzip that round-trips to the payload,
			// not merely a header claiming it.
			zr, err := gzip.NewReader(bytes.NewReader(body))
			require.NoError(t, err, "body must be valid gzip")
			decompressed, err := io.ReadAll(zr)
			require.NoError(t, err)
			assert.Equal(t, payload, decompressed)
		})
	}
}

// TestRawClientCompressesBeforeBaseRoundTripper pins the ordering the auth stack
// depends on: the pool must gzip the body and set Content-Encoding *before* the
// base RoundTripper runs, so a SigV4 signer signs the bytes that actually go on
// the wire. Signing a payload that does not match the transmitted body is the
// failure mode behind #8307, so the ordering deserves an explicit guard.
func TestRawClientCompressesBeforeBaseRoundTripper(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Stands in for the auth/header stack that wraps the real transport.
	spy := &bodyObservingRoundTripper{base: http.DefaultTransport}
	rc, err := newRawClient(t.Context(), spy, rawClientOptions{servers: []string{server.URL}, compressRequestBody: true})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, "/_bulk", strings.NewReader(`{"index":{}}`))
	require.NoError(t, err)
	resp, err := rc.perform(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	assert.Equal(t, "gzip", spy.encoding, "the base RoundTripper must see Content-Encoding already set")
	assert.True(t, bytes.HasPrefix(spy.body, gzipMagic), "the base RoundTripper must see gzip-framed bytes")

	// A signer re-reads the payload through GetBody rather than the consumed Body.
	// If GetBody replayed the original uncompressed bytes, the signature would cover
	// a different payload than the one transmitted — the #8307 failure mode — so
	// assert it yields exactly what the transport sends, not merely that it is set.
	require.NotNil(t, spy.getBody, "GetBody must be populated for a signer to re-read the payload")
	replay, err := spy.getBody()
	require.NoError(t, err)
	replayed, err := io.ReadAll(replay)
	require.NoError(t, err)
	require.NoError(t, replay.Close())
	assert.Equal(t, spy.body, replayed, "GetBody must replay the same compressed bytes the transport sends")
}

// gzipMagic is the two-byte header every gzip stream starts with.
var gzipMagic = []byte{0x1f, 0x8b}

// bodyObservingRoundTripper records what the auth layer would see.
type bodyObservingRoundTripper struct {
	base     http.RoundTripper
	encoding string
	body     []byte
	getBody  func() (io.ReadCloser, error)
}

func (rt *bodyObservingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.encoding = req.Header.Get("Content-Encoding")
	rt.getBody = req.GetBody
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		rt.body = body
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
	return rt.base.RoundTrip(req)
}

// TestBulkIndexerRequestsAreCompressed guards the path that actually regressed in
// #9071. TestNewClientHonorsHTTPCompression covers request(); this covers the
// esutil BulkIndexer, which reaches the pool through Perform instead, so a future
// refactor giving bulk writes their own transport cannot silently drop gzip again.
func TestBulkIndexerRequestsAreCompressed(t *testing.T) {
	var gotEncoding, gotBody atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEncoding.Store(r.Header.Get("Content-Encoding"))
		b, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		gotBody.Store(b)
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write([]byte(`{"took":1,"errors":false,"items":[]}`))
		assert.NoError(t, err)
	}))
	defer server.Close()

	c, err := NewClient(t.Context(), &config.Configuration{
		Servers:         []string{server.URL},
		HTTPCompression: true,
		Version:         uint(es.ElasticV7),
	}, zap.NewNop(), nil)
	require.NoError(t, err)

	b, err := NewBulkIndexer(c, BulkIndexerConfig{}, metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)
	b.Add(BulkItem{Index: "jaeger-span-write", Body: map[string]string{"traceID": "1234567890abcdef"}})
	require.NoError(t, b.Close())

	assert.Equal(t, "gzip", gotEncoding.Load())
	body, ok := gotBody.Load().([]byte)
	require.True(t, ok)
	zr, err := gzip.NewReader(bytes.NewReader(body))
	require.NoError(t, err, "the bulk body must be valid gzip")
	decompressed, err := io.ReadAll(zr)
	require.NoError(t, err)
	assert.Contains(t, string(decompressed), "1234567890abcdef", "the gzip must round-trip to the enqueued document")
}

// TestClientRequestBodyAndTimeout covers request() sending a body under a
// configured per-request timeout.
func TestClientRequestBodyAndTimeout(t *testing.T) {
	var gotBody atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody.Store(string(b))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c, err := NewClient(t.Context(), &config.Configuration{
		Servers:      []string{server.URL},
		QueryTimeout: time.Minute,
		Version:      uint(es.ElasticV7),
	}, zap.NewNop(), nil)
	require.NoError(t, err)
	_, err = c.request(t.Context(), elasticRequest{
		method:   http.MethodPost,
		endpoint: "_bulk",
		body:     []byte("request-payload"),
	})
	require.NoError(t, err)
	assert.Equal(t, "request-payload", gotBody.Load())
}

// TestClientClose covers the full close path: Client.Close forwards to
// rawClient.close, which releases the base transport's idle connections through
// the closeableRoundTripper that GetHTTPRoundTripper installs. It also covers the
// zero-Client case, where there is no transport to close.
func TestClientClose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c, err := NewClient(t.Context(), &config.Configuration{
		Servers: []string{server.URL},
		Version: uint(es.ElasticV7),
	}, zap.NewNop(), nil)
	require.NoError(t, err)
	require.NoError(t, c.Close())

	// A nil *Client (a factory that failed to construct one) must still close cleanly.
	require.NoError(t, (*Client)(nil).Close())
}

// TestTestsOnlyBackendVersion covers the test-only accessor that exposes the
// version resolved at construction, so integration tests need not re-probe.
func TestTestsOnlyBackendVersion(t *testing.T) {
	c, err := NewClient(t.Context(), &config.Configuration{
		Servers: []string{"http://localhost:9200"},
		Version: uint(es.OpenSearch2),
	}, zap.NewNop(), nil)
	require.NoError(t, err)
	assert.Equal(t, es.OpenSearch2, c.TestsOnlyBackendVersion())
	assert.True(t, c.TestsOnlyBackendVersion().IsOpenSearch())
}

// errBodyRoundTripper returns a 200 whose body errors on read.
type errBodyRoundTripper struct{}

func (errBodyRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(errReader{})}, nil
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestClientRequestErrorPaths(t *testing.T) {
	t.Run("invalid method fails request construction", func(t *testing.T) {
		c, err := NewClient(t.Context(), &config.Configuration{
			Servers: []string{"http://localhost:9200"},
			Version: uint(es.ElasticV7),
		}, zap.NewNop(), nil)
		require.NoError(t, err)
		_, err = c.request(t.Context(), elasticRequest{method: "BAD METHOD", endpoint: "x"})
		require.Error(t, err)
	})

	t.Run("response body read error propagates", func(t *testing.T) {
		raw, err := newRawClient(t.Context(), errBodyRoundTripper{}, rawClientOptions{servers: []string{"http://localhost:9200"}})
		require.NoError(t, err)
		c := Client{transport: raw}
		_, err = c.request(t.Context(), elasticRequest{method: http.MethodGet, endpoint: ""})
		require.Error(t, err)
	})
}

// TestNewClientRoundTripperError covers NewClient surfacing an error from
// GetHTTPRoundTripper (here, an invalid basic-auth config).
func TestNewClientRoundTripperError(t *testing.T) {
	_, err := NewClient(t.Context(), &config.Configuration{
		Servers: []string{"http://localhost:9200"},
		Authentication: config.Authentication{
			BasicAuthentication: configoptional.Some(config.BasicAuthentication{
				Username:         "u",
				Password:         "p",
				PasswordFilePath: "/does-not-matter",
			}),
		},
	}, zap.NewNop(), nil)
	require.ErrorContains(t, err, "basic authentication")
}

// TestRawClientSniffingDiscoversNodesOnStart pins that sniffing.enabled makes the
// client query /_nodes/http once at construction. The seed returns an empty node
// set so discovery succeeds and leaves the pool untouched — the point is only that
// the one-shot discovery ran.
func TestRawClientSniffingDiscoversNodesOnStart(t *testing.T) {
	var sawDiscovery atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_nodes/http" {
			sawDiscovery.Store(true)
			_, _ = w.Write([]byte(`{"nodes":{}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	rc, err := newRawClient(t.Context(), http.DefaultTransport, rawClientOptions{
		servers:       []string{server.URL},
		discoverNodes: true,
	})
	require.NoError(t, err)
	require.NotNil(t, rc)
	assert.True(t, sawDiscovery.Load(), "sniffing.enabled must query /_nodes/http at startup")
}

// TestRawClientSniffingDisabledSkipsDiscovery pins the default: with sniffing off,
// the client never queries /_nodes/http.
func TestRawClientSniffingDisabledSkipsDiscovery(t *testing.T) {
	var sawDiscovery atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_nodes/http" {
			sawDiscovery.Store(true)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, err := newRawClient(t.Context(), http.DefaultTransport, rawClientOptions{
		servers: []string{server.URL},
	})
	require.NoError(t, err)
	assert.False(t, sawDiscovery.Load(), "sniffing off must not query /_nodes/http")
}

// TestRawClientSniffingDiscoveryFailureIsFatal pins that a discovery failure fails
// construction rather than silently starting with an unsniffed pool: an operator
// who opted into sniffing should hear about it.
func TestRawClientSniffingDiscoveryFailureIsFatal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_nodes/http" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, err := newRawClient(t.Context(), http.DefaultTransport, rawClientOptions{
		servers:       []string{server.URL},
		discoverNodes: true,
	})
	require.ErrorContains(t, err, "failed to discover Elasticsearch nodes (sniffing)")
}

func TestNewRawClientPoolBuildError(t *testing.T) {
	orig := newPool
	t.Cleanup(func() { newPool = orig })
	newPool = func(...elastictransport.Option) (*elastictransport.Client, error) {
		return nil, errors.New("boom")
	}
	_, err := newRawClient(t.Context(), http.DefaultTransport, rawClientOptions{servers: []string{"http://localhost:9200"}})
	require.ErrorContains(t, err, "failed to build transport pool")
}
