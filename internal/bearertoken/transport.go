// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package bearertoken

import (
	"errors"
	"net/http"
)

// RoundTripper wraps another http.RoundTripper and injects
// an authentication header with a token into requests.
type RoundTripper struct {
	// Transport is the underlying http.RoundTripper being wrapped. Required.
	Transport http.RoundTripper

	// StaticToken is the pre-configured token. Optional.
	StaticToken string

	// AuthScheme is the authentication scheme (e.g., "Bearer", "ApiKey").
	AuthScheme string

	// OverrideFromCtx enables reading token from Context.
	OverrideFromCtx bool
}

// RoundTrip injects the outbound Authorization header with the
// token provided in the inbound request.
func (tr RoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if tr.Transport == nil {
		return nil, errors.New("no http.RoundTripper provided")
	}

	authScheme := tr.AuthScheme
	// Clone the request to avoid modifying the original
	req := r.Clone(r.Context())

	token := tr.StaticToken
	if tr.OverrideFromCtx {
		var ctxToken string
		var ok bool
		if tr.AuthScheme == "ApiKey" {
			ctxToken, ok = APIKeyFromContext(r.Context())
		} else {
			ctxToken, ok = GetBearerToken(r.Context())
		}
		if ok && ctxToken != "" {
			token = ctxToken
		}
	}

	if token != "" {
		req.Header.Set("Authorization", authScheme+" "+token)
	}

	return tr.Transport.RoundTrip(req)
}
