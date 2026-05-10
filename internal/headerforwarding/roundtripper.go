// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package headerforwarding

import "net/http"

type httpRoundTripper struct {
	base http.RoundTripper
}

// NewHTTPRoundTripper returns a RoundTripper that forwards captured header
// values from the request context to outbound HTTP requests.
func NewHTTPRoundTripper(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &httpRoundTripper{base: base}
}

func (t *httpRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	captured := CapturedFromContext(req.Context())
	if len(captured) > 0 {
		req = cloneWithHeaders(req, captured)
	}
	return t.base.RoundTrip(req)
}

func cloneWithHeaders(req *http.Request, captured []CapturedHeader) *http.Request {
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	for _, c := range captured {
		if c.Header == nil || c.Value == "" {
			continue
		}
		if name := c.Header.OutboundName(); name != "" {
			clone.Header.Set(name, c.Value)
		}
	}
	return clone
}
