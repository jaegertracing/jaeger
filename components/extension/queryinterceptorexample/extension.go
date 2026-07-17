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

	"github.com/jaegertracing/jaeger/components/extension/jaegerquery/queryinterceptor"
)

const redactedPlaceholder = "REDACTED"

// interceptor is both an OTel extension (component.Component) and a
// queryinterceptor.Interceptor — the two interfaces a query-interceptor plugin
// must satisfy. It references only public Jaeger packages.
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
func (i *interceptor) OnQuery(_ context.Context, query queryinterceptor.Query) (queryinterceptor.Query, error) {
	for _, key := range i.cfg.DenyQueryAttributes {
		if _, ok := query.Attributes.Get(key); ok {
			i.logger.Debug("rejecting query that filters on a forbidden attribute", zap.String("attribute", key))
			// Wrap ErrRejected so jaeger-query returns a 4xx rather than a 500.
			return query, fmt.Errorf("%w: filtering on attribute %q is not permitted", queryinterceptor.ErrRejected, key)
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
