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

// callerRole returns the caller's role and a context that carries it. A role
// cached by an earlier hook is returned as-is (context unchanged); otherwise the
// role is read from the identity header — which jaeger_query exposes as OTel
// client metadata (requires http.include_metadata) — and cached in the returned
// context, so a later hook (e.g. OnResult after OnQuery, or the next result
// batch) reuses it without re-reading the metadata. The role is "" when the
// metadata has no value.
func (i *interceptor) callerRole(ctx context.Context) (context.Context, string) {
	type roleCacheKey struct{}
	if role, ok := ctx.Value(roleCacheKey{}).(string); ok {
		return ctx, role
	}
	var role string
	if i.cfg.IdentityHeader != "" {
		if values := client.FromContext(ctx).Metadata.Get(i.cfg.IdentityHeader); len(values) > 0 {
			role = values[0]
		}
	}
	return context.WithValue(ctx, roleCacheKey{}, role), role
}

// privileged reports whether the given role is exempt from the restrictions.
func (i *interceptor) privileged(role string) bool {
	for _, r := range i.cfg.PrivilegedRoles {
		if r == role {
			return true
		}
	}
	return false
}

// OnQuery rejects a non-privileged caller's query that filters on a denied
// attribute — the per-caller, pre-query admission hook. It caches the resolved
// role in the returned context so OnResult reuses it without re-reading metadata.
func (i *interceptor) OnQuery(ctx context.Context, query queryinterceptor.Query) (context.Context, queryinterceptor.Query, error) {
	ctx, role := i.callerRole(ctx)
	if i.privileged(role) {
		return ctx, query, nil
	}
	for _, key := range i.cfg.DenyQueryAttributes {
		if _, ok := query.Attributes.Get(key); ok {
			i.logger.Debug("rejecting query that filters on a forbidden attribute",
				zap.String("attribute", key), zap.String("caller_role", role))
			return ctx, query, fmt.Errorf("query interceptor: filtering on attribute %q is not permitted", key)
		}
	}
	return ctx, query, nil
}

// OnResult redacts the configured attributes from every span for non-privileged
// callers — the per-caller, return-path masking hook.
func (i *interceptor) OnResult(ctx context.Context, traces []ptrace.Traces) (context.Context, []ptrace.Traces, error) {
	ctx, role := i.callerRole(ctx)
	if i.privileged(role) || len(i.cfg.RedactAttributes) == 0 {
		return ctx, traces, nil
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
	return ctx, traces, nil
}
