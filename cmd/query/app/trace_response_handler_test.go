// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/jaegertracing/jaeger/pkg/jtracer"
)

func TestTraceResponseHandler(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	jTracer := jtracer.JTracer{OTEL: tracerProvider}
	tracer := jTracer.OTEL.Tracer("instrumentation/trace_response_handler_test")

	emptyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	handlerWithTracing := traceResponseHandler(emptyHandler)
	hanlderWithInstrumentation := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, span := tracer.Start(r.Context(), "operation")
		defer span.End()

		handlerWithTracing.ServeHTTP(w, r)
	})

	server := httptest.NewServer(hanlderWithInstrumentation)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	require.NoError(t, err)

	resp, err := server.Client().Do(req)
	require.NoError(t, err)

	traceResponse := resp.Header.Get("traceresponse")

	// TODO: why isn't this populated?
	require.Equal(t, "", traceResponse)
}
