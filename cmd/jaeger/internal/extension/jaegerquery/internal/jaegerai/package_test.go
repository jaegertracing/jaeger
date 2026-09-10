// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

func TestMain(m *testing.M) {
	// Mirrors what internal/jtracer installs as the global propagator in
	// production before any request is served. Tests exercising
	// injectTraceContextIntoMeta (trace_propagation_test.go) rely on
	// otel.GetTextMapPropagator() actually being this composite rather than
	// the SDK's no-op default, same as the real process.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	testutils.VerifyGoLeaks(m)
}
