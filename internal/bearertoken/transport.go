// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package bearertoken

import (
	"context"
	"errors"
	"net/http"
)

// RoundTripper wraps another http.RoundTripper and injects
// an authentication header with a token into requests.
type RoundTripper struct {
	// Transport is the underlying http.RoundTripper being wrapped. Required.
	Transport http.RoundTripper

	// AuthScheme is the authentication scheme (e.g., "Bearer", "ApiKey").
	AuthScheme string

	// OverrideFromCtx enables reading token from Context.
	OverrideFromCtx bool

	// TokenFn returns the cached token.
	TokenFn func() string

	// FromCtxFn extracts token from context.
	FromCtxFn func(context.Context) (string, bool)
}

// RoundTrip injects the outbound Authorization header with the
// token provided in the inbound request.
func (tr RoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if tr.Transport == nil {
		return nil, errors.New("no http.RoundTripper provided")
	}
	// Clone the request to avoid modifying the original
	req := r.Clone(r.Context())
	token := ""

	// Get token from TokenFn if available
	if tr.TokenFn != nil {
		token = tr.TokenFn()
	}

	// Check context for token override
	if tr.OverrideFromCtx && tr.FromCtxFn != nil {
		ctxToken, ok := tr.FromCtxFn(r.Context())
		if ok && ctxToken != "" {
			token = ctxToken
		}
	}

	if token != "" {
		req.Header.Set("Authorization", tr.AuthScheme+" "+token)
	}

	return tr.Transport.RoundTrip(req)
}
