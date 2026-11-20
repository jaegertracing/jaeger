// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package headerforwarding

import (
	"context"
	"net/http"
)

// NewHTTPClientRoundTripper returns an http.RoundTripper that forwards header values
// captured in the request context (via HTTPServerMiddleware or the gRPC server
// interceptors) onto outbound HTTP requests, keyed by ForwardedHeader.HTTPOutboundName
// (falling back to HTTPName when unset).
//
// It is the HTTP-backend analog of NewUnaryClientInterceptor / NewStreamClientInterceptor
// and is intended for HTTP-based storage backends such as Elasticsearch / OpenSearch.
//
// If base is nil, http.DefaultTransport is used.
func NewHTTPClientRoundTripper(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &httpClientRoundTripper{base: base}
}

type httpClientRoundTripper struct {
	base http.RoundTripper
}

func (rt *httpClientRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(CapturedFromContext(req.Context())) == 0 {
		return rt.base.RoundTrip(req)
	}
	// RoundTripper contract: do not mutate the inbound *http.Request.
	cloned := req.Clone(req.Context())
	SetForwardedHTTPHeaders(req.Context(), cloned.Header)
	return rt.base.RoundTrip(cloned)
}

// SetForwardedHTTPHeaders sets the header values captured in ctx on header,
// each under its outbound HTTP name. It is for an outbound HTTP request that
// does not go through an http.RoundTripper, such as a WebSocket handshake;
// NewHTTPClientRoundTripper applies the same rule to ordinary clients.
func SetForwardedHTTPHeaders(ctx context.Context, header http.Header) {
	for _, c := range CapturedFromContext(ctx) {
		if c.Value == "" || c.Header == nil {
			continue
		}
		name := c.Header.outboundHTTPName()
		if name == "" {
			continue
		}
		header.Set(name, c.Value)
	}
}
