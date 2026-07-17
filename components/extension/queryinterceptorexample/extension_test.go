// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package queryinterceptorexample

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/client"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/components/extension/jaegerquery/queryinterceptor"
)

const identityHeader = "x-jaeger-caller-role"

// restrictedCfg privileges role "admin" and restricts everyone else.
func restrictedCfg() *Config {
	return &Config{
		IdentityHeader:      identityHeader,
		PrivilegedRoles:     []string{"admin"},
		DenyQueryAttributes: []string{"prompt"},
		RedactAttributes:    []string{"prompt"},
	}
}

// ctxWithRole returns a context carrying the caller role in client metadata, as
// jaeger_query's http.include_metadata does from the request header.
func ctxWithRole(role string) context.Context {
	md := client.NewMetadata(map[string][]string{identityHeader: {role}})
	return client.NewContext(context.Background(), client.Info{Metadata: md})
}

func queryWithAttr(key, val string) queryinterceptor.Query {
	q := queryinterceptor.Query{Attributes: pcommon.NewMap()}
	q.Attributes.PutStr(key, val)
	return q
}

func tracesWithSpanAttrs(kv map[string]string) []ptrace.Traces {
	td := ptrace.NewTraces()
	span := td.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	for k, v := range kv {
		span.Attributes().PutStr(k, v)
	}
	return []ptrace.Traces{td}
}

func TestOnQuery_DeniesForbiddenAttributeForRestrictedCaller(t *testing.T) {
	i := newInterceptor(restrictedCfg(), zap.NewNop())

	// Restricted caller (unknown role) is rejected for a forbidden attribute.
	_, err := i.OnQuery(ctxWithRole("viewer"), queryWithAttr("prompt", "x"))
	require.ErrorContains(t, err, `filtering on attribute "prompt" is not permitted`)

	// Same query, but the privileged caller is admitted unchanged.
	allowed := queryWithAttr("prompt", "x")
	got, err := i.OnQuery(ctxWithRole("admin"), allowed)
	require.NoError(t, err)
	assert.Equal(t, allowed, got)

	// A query that touches no forbidden attribute is always admitted.
	benign := queryWithAttr("service", "checkout")
	got, err = i.OnQuery(ctxWithRole("viewer"), benign)
	require.NoError(t, err)
	assert.Equal(t, benign, got)

	// A caller with no identity metadata at all is treated as restricted.
	_, err = i.OnQuery(context.Background(), queryWithAttr("prompt", "x"))
	require.ErrorContains(t, err, `filtering on attribute "prompt" is not permitted`)
}

func TestOnResult_RedactsForRestrictedCallerOnly(t *testing.T) {
	i := newInterceptor(restrictedCfg(), zap.NewNop())

	// Restricted caller: the configured attribute is redacted, others untouched.
	restricted := tracesWithSpanAttrs(map[string]string{"prompt": "my secret", "service": "checkout"})
	out, err := i.OnResult(ctxWithRole("viewer"), restricted)
	require.NoError(t, err)
	attrs := out[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	prompt, _ := attrs.Get("prompt")
	assert.Equal(t, redactedPlaceholder, prompt.Str())
	service, _ := attrs.Get("service")
	assert.Equal(t, "checkout", service.Str(), "non-redacted attributes are untouched")

	// Privileged caller: nothing is redacted.
	privileged := tracesWithSpanAttrs(map[string]string{"prompt": "my secret"})
	out, err = i.OnResult(ctxWithRole("admin"), privileged)
	require.NoError(t, err)
	attrs = out[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	prompt, _ = attrs.Get("prompt")
	assert.Equal(t, "my secret", prompt.Str(), "privileged caller sees the value unredacted")
}

func TestOnResult_NoConfigIsPassThrough(t *testing.T) {
	i := newInterceptor(&Config{}, zap.NewNop())
	batch := tracesWithSpanAttrs(map[string]string{"prompt": "kept"})
	out, err := i.OnResult(context.Background(), batch)
	require.NoError(t, err)
	attrs := out[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	prompt, _ := attrs.Get("prompt")
	assert.Equal(t, "kept", prompt.Str())
}

func TestFactory_CreatesInterceptorExtension(t *testing.T) {
	f := NewFactory()
	require.NotNil(t, f.CreateDefaultConfig())

	ext, err := f.Create(context.Background(), extension.Settings{
		ID:                component.NewID(componentType),
		TelemetrySettings: componenttest.NewNopTelemetrySettings(),
	}, f.CreateDefaultConfig())
	require.NoError(t, err)

	require.NoError(t, ext.Start(context.Background(), componenttest.NewNopHost()))
	require.NoError(t, ext.Shutdown(context.Background()))
	_, ok := ext.(queryinterceptor.Interceptor)
	assert.True(t, ok, "extension must implement queryinterceptor.Interceptor")
}
