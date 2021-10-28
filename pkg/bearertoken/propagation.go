// Copyright (c) 2019 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bearertoken

import (
	"context"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

type contextKey string

// Key is the string literal used internally in the implementation of this context.
const Key = "bearer.token"
const bearerToken = contextKey(Key)

// StoragePropagationKey is a key for viper configuration to pass this option to storage plugins.
const StoragePropagationKey = "storage.propagate.token"

// ContextWithBearerToken set bearer token in context.
func ContextWithBearerToken(ctx context.Context, token string) context.Context {
	if token == "" {
		return ctx
	}
	return context.WithValue(ctx, bearerToken, token)
}

// GetBearerToken from context, or empty string if there is no token.
func GetBearerToken(ctx context.Context) (string, bool) {
	val, ok := ctx.Value(bearerToken).(string)
	return val, ok
}

// PropagationHandler returns a http.Handler containing the logic to extract
// the Authorization token from the http.Request and inserts it into the http.Request
// context for easier access to the request token via GetBearerToken for bearer token
// propagation use cases.
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
			if len(headerValue) == 2 {
				// Make sure we only capture bearer token , not other types like Basic auth.
				if headerValue[0] == "Bearer" {
					token = headerValue[1]
				}
			} else if len(headerValue) == 1 {
				// Treat the entire value as a token.
				token = authHeaderValue
			} else {
				logger.Warn("Invalid authorization header value, skipping token propagation")
			}
			h.ServeHTTP(w, r.WithContext(ContextWithBearerToken(ctx, token)))
		} else {
			h.ServeHTTP(w, r.WithContext(ctx))
		}
	})
}
