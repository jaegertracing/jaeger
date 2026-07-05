// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package esclient

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

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
