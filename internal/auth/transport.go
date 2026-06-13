// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// Method represents a single authentication method configuration
type Method struct {
	// Scheme is the authentication scheme (e.g., "Bearer")
	Scheme string

	// TokenFn returns the authentication token
	TokenFn func() string

	// FromCtx extracts token from context
	FromCtx func(context.Context) (string, bool)
}

// RoundTripper wraps another http.RoundTripper and injects
// an authentication header with  token into requests.
type RoundTripper struct {
	// Transport is the underlying http.RoundTripper being wrapped. Required.
	Transport http.RoundTripper
	Auths     []Method
}

// RoundTrip injects the outbound Authorization header with the
// token provided in the inbound request.
func (tr RoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if tr.Transport == nil {
		return nil, errors.New("no http.RoundTripper provided")
	}

	req := r.Clone(r.Context())

	for _, auth := range tr.Auths {
		token := ""

		// Get token from context if available
		if auth.FromCtx != nil {
			if t, ok := auth.FromCtx(r.Context()); ok {
				token = t
			}
		}

		// Fall back to TokenFn if no token from context
		if token == "" && auth.TokenFn != nil {
			token = auth.TokenFn()
		}

		// Add Authorization header if we have a token
		if token != "" {
			req.Header.Add("Authorization", fmt.Sprintf("%s %s", auth.Scheme, token))
		}
	}

	return tr.Transport.RoundTrip(req)
}
