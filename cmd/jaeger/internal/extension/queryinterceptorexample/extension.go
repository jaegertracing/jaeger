// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package queryinterceptorexample

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	// Imported through the public module-root path, exactly as a third-party
	// OCB extension would — no Jaeger-internal packages are referenced.
	"github.com/jaegertracing/jaeger/components/extension/queryinterceptor"
)

const redactedPlaceholder = "REDACTED"

// interceptor is both an OTel extension (component.Component) and a
// queryinterceptor.Interceptor — the two interfaces a query-interceptor plugin
// must satisfy.
type interceptor struct {
	cfg    *Config
	logger *zap.Logger
}

var (
	_ extension.Extension          = (*interceptor)(nil)
	_ queryinterceptor.Interceptor = (*interceptor)(nil)
)

func newInterceptor(cfg *Config, logger *zap.Logger) *interceptor {
	return &interceptor{cfg: cfg, logger: logger}
}

func (*interceptor) Start(context.Context, component.Host) error { return nil }

func (*interceptor) Shutdown(context.Context) error { return nil }

// OnQuery rejects a query that filters on any denied attribute — the pre-query
// admission hook.
func (i *interceptor) OnQuery(_ context.Context, query queryinterceptor.TraceQueryParams) (queryinterceptor.TraceQueryParams, error) {
	for _, key := range i.cfg.DenyQueryAttributes {
		if _, ok := query.Attributes.Get(key); ok {
			i.logger.Debug("rejecting query that filters on a forbidden attribute", zap.String("attribute", key))
			return query, fmt.Errorf("query interceptor: filtering on attribute %q is not permitted", key)
		}
	}
	return query, nil
}

// OnResult redacts the configured attributes from every span in the batch — the
// return-path masking hook.
func (i *interceptor) OnResult(_ context.Context, traces []ptrace.Traces) ([]ptrace.Traces, error) {
	if len(i.cfg.RedactAttributes) == 0 {
		return traces, nil
	}
	for _, td := range traces {
		resourceSpans := td.ResourceSpans()
		for ri := 0; ri < resourceSpans.Len(); ri++ {
			scopeSpans := resourceSpans.At(ri).ScopeSpans()
			for si := 0; si < scopeSpans.Len(); si++ {
				spans := scopeSpans.At(si).Spans()
				for spi := 0; spi < spans.Len(); spi++ {
					attrs := spans.At(spi).Attributes()
					for _, key := range i.cfg.RedactAttributes {
						if _, ok := attrs.Get(key); ok {
							attrs.PutStr(key, redactedPlaceholder)
						}
					}
				}
			}
		}
	}
	return traces, nil
}
