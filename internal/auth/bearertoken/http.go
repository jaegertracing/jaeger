// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package bearertoken

import (
	"net/http"
	"strings"

	"go.uber.org/zap"
)

// PropagationHandler returns a http.Handler containing the logic to extract
// the Bearer token from the Authorization header of the http.Request and insert it into request.Context
// for propagation. The token can be accessed via GetBearerToken.
func PropagationHandler(logger *zap.Logger, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		authHeaderValue := r.Header.Get("Authorization")
		// If no Authorization header is present, try with X-Forwarded-Access-Token
		if authHeaderValue == "" {
			authHeaderValue = r.Header.Get("X-Forwarded-Access-Token")
		}
		if authHeaderValue != "" {
			headerValue := strings.Split(authHeaderValue, " ")
			token := ""
			switch {
			case len(headerValue) == 2:
				// Make sure we only capture bearer token , not other types like Basic auth.
				if headerValue[0] == "Bearer" {
					token = headerValue[1]
				}
			case len(headerValue) == 1:
				// Treat the entire value as a token.
				token = authHeaderValue
			default:
				logger.Warn("Invalid authorization header value, skipping token propagation")
			}
			h.ServeHTTP(w, r.WithContext(ContextWithBearerToken(ctx, token)))
		} else {
			h.ServeHTTP(w, r.WithContext(ctx))
		}
	})
}
