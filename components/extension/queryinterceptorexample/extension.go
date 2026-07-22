// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package queryinterceptorexample

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/client"
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

// callerRole reads the caller's role from the identity header, which
// jaeger_query exposes on the context as OTel client metadata (requires
// http.include_metadata). Returns "" when absent.
func (i *interceptor) callerRole(ctx context.Context) string {
	if i.cfg.IdentityHeader == "" {
		return ""
	}
	values := client.FromContext(ctx).Metadata.Get(i.cfg.IdentityHeader)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// privileged reports whether the caller's role is exempt from the restrictions.
func (i *interceptor) privileged(ctx context.Context) bool {
	role := i.callerRole(ctx)
	for _, r := range i.cfg.PrivilegedRoles {
		if r == role {
			return true
		}
	}
	return false
}

// OnQuery rejects a non-privileged caller's query that filters on a denied
// attribute — the per-caller, pre-query admission hook.
func (i *interceptor) OnQuery(ctx context.Context, query queryinterceptor.Query) (queryinterceptor.Query, error) {
	if i.privileged(ctx) {
		return query, nil
	}
	for _, key := range i.cfg.DenyQueryAttributes {
		if _, ok := query.Attributes.Get(key); ok {
			i.logger.Debug("rejecting query that filters on a forbidden attribute",
				zap.String("attribute", key), zap.String("caller_role", i.callerRole(ctx)))
			return query, fmt.Errorf("query interceptor: filtering on attribute %q is not permitted", key)
		}
	}
	return query, nil
}

// OnResult redacts the configured attributes from every span for non-privileged
// callers — the per-caller, return-path masking hook.
func (i *interceptor) OnResult(ctx context.Context, traces []ptrace.Traces) ([]ptrace.Traces, error) {
	if i.privileged(ctx) || len(i.cfg.RedactAttributes) == 0 {
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
