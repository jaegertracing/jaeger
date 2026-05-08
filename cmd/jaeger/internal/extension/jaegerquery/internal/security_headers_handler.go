// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"net/http"
)

// securityHeadersHandler adds standard security headers to the response.
// It is intended to be used for UI assets, not API endpoints.
func securityHeadersHandler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent browsers from MIME-sniffing a response away from the declared content-type.
		if w.Header().Get("X-Content-Type-Options") == "" {
			w.Header().Set("X-Content-Type-Options", "nosniff")
		}

		// Content Security Policy (CSP)
		// This is a baseline policy that allows assets from 'self' and common UI needs.
		// - default-src 'none': block everything by default
		// - script-src 'self' 'unsafe-inline' 'unsafe-eval': allow scripts from same origin, inline scripts (JAEGER_CONFIG), and eval (often used by UI frameworks/bundlers)
		// - style-src 'self' 'unsafe-inline': allow styles from same origin and inline styles (used by React/CSS-in-JS)
		// - font-src 'self' data: allow fonts from same origin and data URIs (e.g. for icon fonts)
		// - img-src 'self' data: allow images from same origin and data URIs
		// - connect-src 'self': allow AJAX/Websocket to same origin
		// - base-uri 'self': restrict <base> tag to same origin
		// - form-action 'self': restrict form submissions to same origin
		// Note: frame-ancestors is omitted intentionally to allow embedding in tools like Grafana or Backstage.
		if w.Header().Get("Content-Security-Policy") == "" {
			csp := "default-src 'none'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; font-src 'self' data:; img-src 'self' data:; connect-src 'self'; base-uri 'self'; form-action 'self';"
			w.Header().Set("Content-Security-Policy", csp)
		}

		handler.ServeHTTP(w, r)
	})
}
