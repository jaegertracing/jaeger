// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package headerforwarding

import "net/http"

// HTTPServerMiddleware returns an http.Handler that extracts the configured headers from
// inbound HTTP requests and stores them in the request context for downstream propagation.
func HTTPServerMiddleware(headers []ForwardedHeader, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured := make([]CapturedHeader, 0, len(headers))
		for i := range headers {
			if v := r.Header.Get(headers[i].HTTPName); v != "" {
				captured = append(captured, CapturedHeader{Header: &headers[i], Value: v})
			}
		}
		if len(captured) > 0 {
			r = r.WithContext(ContextWithCaptured(r.Context(), captured))
		}
		next.ServeHTTP(w, r)
	})
}
