// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTraceResponseHandler(t *testing.T) {
	emptyHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte{})
	})
	handler := traceResponseHandler(emptyHandler)

	exporter := tracetest.NewInMemoryExporter()
	tracerProvider := trace.NewTracerProvider(
		trace.WithSyncer(exporter),
		trace.WithSampler(trace.AlwaysSample()),
	)
	tracer := tracerProvider.Tracer("test-tracer")
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:8080", nil)
	require.NoError(t, err)

	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	traceResponse := w.Header().Get("traceresponse")
	parts := strings.Split(traceResponse, "-")
	require.Len(t, parts, 4)
}
