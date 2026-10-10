// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/internal/headerforwarding"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

// The middlewares between otelhttp and the mux hand the mux a shallow copy of the
// request, so the pattern the mux writes onto it is invisible to otelhttp, which
// then leaves the span with the name it was given before routing — an empty one,
// under the query server's span name formatter. Each case below turns on one of
// those middlewares and asserts the span is still named after the route.
func TestQueryServerSpanIsNamedAfterRoute(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*QueryOptions)
		headers   map[string]string
		spanName  string
		route     string
	}{
		{
			name:     "no middleware",
			spanName: "/api/v3/services",
			route:    "/api/v3/services",
		},
		{
			name: "bearer token propagation",
			configure: func(opts *QueryOptions) {
				opts.BearerTokenPropagation = true
			},
			headers:  map[string]string{"Authorization": "Bearer token"},
			spanName: "/api/v3/services",
			route:    "/api/v3/services",
		},
		{
			name: "header forwarding, configured header present",
			configure: func(opts *QueryOptions) {
				opts.HeaderForwarding = []headerforwarding.ForwardedHeader{
					{HTTPName: "x-grafana-user", Role: headerforwarding.RoleUsername},
				}
			},
			headers:  map[string]string{"x-grafana-user": "alice"},
			spanName: "/api/v3/services",
			route:    "/api/v3/services",
		},
		{
			name: "header forwarding, configured header absent",
			configure: func(opts *QueryOptions) {
				opts.HeaderForwarding = []headerforwarding.ForwardedHeader{
					{HTTPName: "x-grafana-user", Role: headerforwarding.RoleUsername},
				}
			},
			spanName: "/api/v3/services",
			route:    "/api/v3/services",
		},
		{
			name: "tenancy",
			configure: func(opts *QueryOptions) {
				opts.Tenancy = tenancy.Options{Enabled: true}
			},
			headers:  map[string]string{"x-tenant": "acme"},
			spanName: "/api/v3/services",
			route:    "/api/v3/services",
		},
		{
			name: "all of them at once",
			configure: func(opts *QueryOptions) {
				opts.BearerTokenPropagation = true
				opts.HeaderForwarding = []headerforwarding.ForwardedHeader{
					{HTTPName: "x-grafana-user", Role: headerforwarding.RoleUsername},
				}
				opts.Tenancy = tenancy.Options{Enabled: true}
			},
			headers: map[string]string{
				"Authorization":  "Bearer token",
				"x-grafana-user": "alice",
				"x-tenant":       "acme",
			},
			spanName: "/api/v3/services",
			route:    "/api/v3/services",
		},
		{
			// The span name drops the base path, so that an endpoint is named the
			// same however the server is mounted, but http.route is the real route.
			name: "base path",
			configure: func(opts *QueryOptions) {
				opts.BasePath = "/jaeger"
				opts.Tenancy = tenancy.Options{Enabled: true}
			},
			headers:  map[string]string{"x-tenant": "acme"},
			spanName: "/api/v3/services",
			route:    "/jaeger/api/v3/services",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opts := &QueryOptions{
				HTTP: confighttp.ServerConfig{
					NetAddr: confignet.AddrConfig{Endpoint: ":0", Transport: confignet.TransportTypeTCP},
				},
				GRPC: configgrpc.ServerConfig{
					NetAddr: confignet.AddrConfig{Endpoint: ":0", Transport: confignet.TransportTypeTCP},
				},
			}
			if test.configure != nil {
				test.configure(opts)
			}

			spans, rm := serveAndCollect(t, opts, opts.BasePath+"/api/v3/services", test.headers)

			require.Len(t, spans, 1)
			assert.Equal(t, test.spanName, spans[0].Name)
			assert.Equal(t, test.route, attributeValue(t, spans[0].Attributes, "http.route"),
				"http.route on the server span")
			assert.Equal(t, test.route, requestDurationRoute(t, rm),
				"http.route on the http.server.request.duration metric")
		})
	}
}

// serveAndCollect starts a real query server, issues one GET against it, and returns
// the spans and metrics it recorded. Going through NewServer is what makes this a
// regression test: the otelhttp wrapping and the span name formatter it runs under
// come from confighttp, and it is their interaction with the query server's own
// middlewares that broke.
func serveAndCollect(
	t *testing.T,
	opts *QueryOptions,
	path string,
	headers map[string]string,
) ([]tracetest.SpanStub, metricdata.ResourceMetrics) {
	spanExporter := tracetest.NewInMemoryExporter()
	tp := tracesdk.NewTracerProvider(
		tracesdk.WithSyncer(spanExporter),
		tracesdk.WithSampler(tracesdk.AlwaysSample()),
	)
	t.Cleanup(func() { require.NoError(t, tp.Shutdown(context.Background())) })

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { require.NoError(t, mp.Shutdown(context.Background())) })

	telset := initTelSet(zaptest.NewLogger(t), tp)
	telset.MeterProvider = mp

	querySvc := makeQuerySvc()
	server, err := NewServer(context.Background(), querySvc.qs, nil, opts,
		nilBackendCaps, tenancy.NewManager(&opts.Tenancy), telset)
	require.NoError(t, err)
	require.NoError(t, server.Start(context.Background()))
	t.Cleanup(func() { require.NoError(t, server.Close()) })

	req, err := http.NewRequest(http.MethodGet, "http://"+server.HTTPAddr()+path, http.NoBody)
	require.NoError(t, err)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := (&http.Client{}).Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	return spanExporter.GetSpans(), rm
}

// requestDurationRoute returns the http.route recorded on the single
// http.server.request.duration data point.
func requestDurationRoute(t *testing.T, rm metricdata.ResourceMetrics) string {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "http.server.request.duration" {
				continue
			}
			hist, ok := m.Data.(metricdata.Histogram[float64])
			require.True(t, ok, "unexpected aggregation %T", m.Data)
			require.Len(t, hist.DataPoints, 1)
			return attributeValue(t, hist.DataPoints[0].Attributes.ToSlice(), "http.route")
		}
	}
	t.Fatal("http.server.request.duration was not recorded")
	return ""
}

func attributeValue(t *testing.T, attrs []attribute.KeyValue, key string) string {
	var found []string
	for _, a := range attrs {
		if string(a.Key) == key {
			found = append(found, a.Value.AsString())
		}
	}
	require.Len(t, found, 1, "%s in %v", key, attrs)
	return found[0]
}

func TestRouteFromPattern(t *testing.T) {
	tests := []struct {
		pattern string
		route   string
	}{
		{pattern: "GET /api/v3/services", route: "/api/v3/services"},
		{pattern: "/api/", route: "/api/"},
		{pattern: "", route: ""},
	}
	for _, test := range tests {
		t.Run(test.pattern, func(t *testing.T) {
			assert.Equal(t, test.route, routeFromPattern(test.pattern))
		})
	}
}

func TestSpanNameForRoute(t *testing.T) {
	tests := []struct {
		name     string
		route    string
		basePath string
		spanName string
	}{
		{name: "no base path", route: "/api/v3/services", spanName: "/api/v3/services"},
		{name: "root base path", route: "/api/v3/services", basePath: "/", spanName: "/api/v3/services"},
		{name: "base path", route: "/jaeger/api/v3/services", basePath: "/jaeger", spanName: "/api/v3/services"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.spanName, spanNameForRoute(test.route, test.basePath))
		})
	}
}
