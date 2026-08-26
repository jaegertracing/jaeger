// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"net/http"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"
)

// routeTagHandler names the server span after the route the mux matched and records that
// route as http.route, on the span and on the http.server.request.duration metric. It has
// to wrap the mux directly: http.ServeMux writes the matched pattern onto the *http.Request
// it was handed, and the middlewares above hand it a shallow copy, so this is the only
// position where reading that pattern is guaranteed to work.
//
// otelhttp derives the same name and route from the pattern, but off the request it passed
// down, which is the original that those middlewares copied — it finds no pattern there and
// leaves the span nameless. Setting these on the span and the metric labeler instead of the
// request keeps them independent of what the middlewares above do with the request.
func routeTagHandler(basePath string, mux http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r)

		route := routeFromPattern(r.Pattern)
		if route == "" {
			return
		}
		routeAttr := semconv.HTTPRoute(route)
		span := trace.SpanFromContext(r.Context())
		span.SetName(spanNameForRoute(route, basePath))
		span.SetAttributes(routeAttr)
		if labeler, ok := otelhttp.LabelerFromContext(r.Context()); ok {
			labeler.Add(routeAttr)
		}
	})
}

// routeFromPattern strips the optional method prefix from an http.ServeMux pattern,
// leaving the route as OpenTelemetry defines it. Returns "" when no route matched.
func routeFromPattern(pattern string) string {
	if i := strings.IndexByte(pattern, '/'); i >= 0 {
		return pattern[i:]
	}
	return ""
}

// spanNameForRoute names a span after the route with the base path removed, so that
// an endpoint is named the same however the server is mounted.
func spanNameForRoute(route, basePath string) string {
	if basePath != "" && basePath != "/" {
		return strings.TrimPrefix(route, basePath)
	}
	return route
}
