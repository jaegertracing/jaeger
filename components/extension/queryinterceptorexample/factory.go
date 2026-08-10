// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package queryinterceptorexample is a demonstration OTel extension that
// implements the public queryinterceptor.Interceptor contract. It shows that
// the Option-D extension point is an ordinary OTel extension — added to the
// config's extensions list, referenced by jaeger_query.query_interceptors, and
// buildable into a custom Jaeger via OCB — not a fork. The logic is deliberately
// trivial (static deny/redact lists); the point is the plumbing, not the policy.
package queryinterceptorexample

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
)

var componentType = component.MustNewType("query_interceptor_example")

// NewFactory returns a factory for the example query-interceptor extension.
func NewFactory() extension.Factory {
	return extension.NewFactory(
		componentType,
		func() component.Config { return &Config{} },
		func(_ context.Context, set extension.Settings, cfg component.Config) (extension.Extension, error) {
			return newInterceptor(cfg.(*Config), set.TelemetrySettings.Logger), nil
		},
		component.StabilityLevelDevelopment,
	)
}
