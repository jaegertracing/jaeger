// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	rc, err := newRawClient([]string{server1.URL, server2.URL}, base)
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

func TestNewRawClientInvalidURL(t *testing.T) {
	tests := map[string]string{
		"unparseable":    "http://host:notaport",
		"missing scheme": "localhost:9200",
		"bare host":      "localhost",
	}
	for name, server := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := newRawClient([]string{server}, http.DefaultTransport)
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

	rc, err := newRawClient([]string{server.URL + "/es"}, http.DefaultTransport)
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

	strict, err := NewClient([]string{server.URL}, &tls.Config{}, "", 0)
	require.NoError(t, err)
	_, err = strict.request(context.Background(), elasticRequest{method: http.MethodGet, endpoint: ""})
	require.Error(t, err, "an untrusted self-signed cert must fail TLS verification")

	insecure, err := NewClient([]string{server.URL}, &tls.Config{InsecureSkipVerify: true}, "", 0)
	require.NoError(t, err)
	_, err = insecure.request(context.Background(), elasticRequest{method: http.MethodGet, endpoint: ""})
	require.NoError(t, err, "InsecureSkipVerify must be honored")
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

	c, err := NewClient([]string{server.URL}, nil, "", time.Minute)
	require.NoError(t, err)
	_, err = c.request(context.Background(), elasticRequest{
		method:   http.MethodPost,
		endpoint: "_bulk",
		body:     []byte(`{"index":{}}`),
	})
	require.NoError(t, err)
	assert.Equal(t, `{"index":{}}`, gotBody.Load())
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
		c, err := NewClient([]string{"http://localhost:9200"}, nil, "", 0)
		require.NoError(t, err)
		_, err = c.request(context.Background(), elasticRequest{method: "BAD METHOD", endpoint: "x"})
		require.Error(t, err)
	})

	t.Run("response body read error propagates", func(t *testing.T) {
		raw, err := newRawClient([]string{"http://localhost:9200"}, errBodyRoundTripper{})
		require.NoError(t, err)
		c := Client{transport: raw}
		_, err = c.request(context.Background(), elasticRequest{method: http.MethodGet, endpoint: ""})
		require.Error(t, err)
	})
}
