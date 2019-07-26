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

package app

import (
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/storage/spanstore"
)

func bearerTokenPropagationHandler(logger *zap.Logger, h http.Handler) http.Handler {
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
				// Tread all value as a token
				token = authHeaderValue
			} else {
				logger.Warn("Invalid authorization header value, skipping token propagation")
			}
			h.ServeHTTP(w, r.WithContext(spanstore.ContextWithBearerToken(ctx, token)))
		} else {
			h.ServeHTTP(w, r.WithContext(ctx))
		}
	})

}
