// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package bearertoken

import (
	"errors"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

var errInvalidAuthHeader = errors.New("invalid authorization header value")

// PropagationHandler returns a http.Handler containing the logic to extract
// the Bearer token from the Authorization header of the http.Request and insert it into request.Context
// for propagation. The token can be accessed via GetBearerToken.
func PropagationHandler(logger *zap.Logger, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := TokenFromHTTPHeader(r.Header)
		if err != nil {
			logger.Warn("Invalid authorization header value, skipping token propagation")
		}
		h.ServeHTTP(w, r.WithContext(ContextWithBearerToken(r.Context(), token)))
	})
}

// TokenFromHTTPHeader returns the token PropagationHandler propagates for a
// request with the given header, or "" when it propagates none.
func TokenFromHTTPHeader(header http.Header) (string, error) {
	authHeaderValue := header.Get("Authorization")
	// If no Authorization header is present, try with X-Forwarded-Access-Token
	if authHeaderValue == "" {
		authHeaderValue = header.Get("X-Forwarded-Access-Token")
	}
	if authHeaderValue == "" {
		return "", nil
	}
	headerValue := strings.Split(authHeaderValue, " ")
	switch {
	case len(headerValue) == 2:
		// Make sure we only capture bearer token , not other types like Basic auth.
		if headerValue[0] == "Bearer" {
			return headerValue[1], nil
		}
		return "", nil
	case len(headerValue) == 1:
		// Treat the entire value as a token.
		return authHeaderValue, nil
	default:
		return "", errInvalidAuthHeader
	}
}
