// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package bearertoken

import (
	"context"
	"errors"
	"net/http"
	"net/url"
)

// TokenFromContextFunc is a function that retrieves a token from context.
type TokenFromContextFunc func(ctx context.Context) (string, bool)

// cloneRequest returns a clone of the provided request for modification.
func cloneRequest(r *http.Request) *http.Request {
	// Shallow copy of the struct
	clone := new(http.Request)
	*clone = *r

	// Deep copy of the URL
	if r.URL != nil {
		cloneURL := new(url.URL)
		*cloneURL = *r.URL
		clone.URL = cloneURL
	}

	// Deep copy of the Header
	clone.Header = make(http.Header, len(r.Header))
	for k, v := range r.Header {
		clone.Header[k] = append([]string(nil), v...)
	}

	return clone
}

// RoundTripper wraps another http.RoundTripper and injects
// an authentication header with a token into requests.
type RoundTripper struct {
	// Transport is the underlying http.RoundTripper being wrapped. Required.
	Transport http.RoundTripper

	// StaticToken is the pre-configured token. Optional.
	StaticToken string

	// AuthScheme is the authentication scheme (e.g., "Bearer", "ApiKey").
	// Defaults to "Bearer" if not specified.
	AuthScheme string

	// OverrideFromCtx enables reading token from Context.
	OverrideFromCtx bool

	// TokenFromContext is a function that retrieves a token from context.
	// If not specified and OverrideFromCtx is true, defaults to GetBearerToken.
	TokenFromContext TokenFromContextFunc
}

// RoundTrip injects the outbound Authorization header with the
// token provided in the inbound request.
func (tr RoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if tr.Transport == nil {
		return nil, errors.New("no http.RoundTripper provided")
	}

	// Use the TokenFromContext function or default to GetBearerToken
	tokenFromCtx := tr.TokenFromContext
	if tokenFromCtx == nil && tr.OverrideFromCtx {
		tokenFromCtx = GetBearerToken
	}

	// Default AuthScheme to "Bearer" if not specified
	authScheme := tr.AuthScheme
	if authScheme == "" {
		authScheme = "Bearer"
	}

	// Clone the request to avoid modifying the original
	req := cloneRequest(r)

	token := tr.StaticToken
	if tr.OverrideFromCtx && tokenFromCtx != nil {
		if ctxToken, ok := tokenFromCtx(req.Context()); ok && ctxToken != "" {
			token = ctxToken
		}
	}

	if token != "" {
		req.Header.Set("Authorization", authScheme+" "+token)
	}

	return tr.Transport.RoundTrip(req)
}
