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
// to wrap the mux directly, because http.ServeMux writes the matched pattern onto whichever
// *http.Request it receives, and the middlewares between otelhttp and the mux pass the mux
// a shallow copy instead of the request they were given — so this handler holds the only
// request that the mux will have written a pattern onto.
//
// otelhttp derives the same name and route from that pattern, but reads it off its own
// request, the one those middlewares copied from, where the pattern never appears. Finding
// none, otelhttp leaves the span nameless. Setting the name and the route on the span and
// the metric labeler, rather than back onto a request, keeps them independent of whatever
// the middlewares above do with the request.
//
// Both are bound late, once the handler has returned, so anything that runs at span start
// sees neither: a head sampler and a SpanProcessor's OnStart get the empty name the span
// was created with, and only the exported span carries the route. That is inherent, since
// the route is not knowable until the mux has matched. otelhttp has the same limitation now
// that it reads r.Pattern; the WithRouteTag it replaced took the route as a literal at
// registration time and could set it before the handler ran, but never set the name.
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
