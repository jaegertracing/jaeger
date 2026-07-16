// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package queryinterceptorexample

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/components/extension/jaegerquery/queryinterceptor"
)

func queryWithAttr(key, val string) queryinterceptor.TraceQueryParams {
	q := queryinterceptor.TraceQueryParams{Attributes: pcommon.NewMap()}
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

func TestOnQuery_DeniesForbiddenAttribute(t *testing.T) {
	i := newInterceptor(&Config{DenyQueryAttributes: []string{"prompt"}}, zap.NewNop())

	_, err := i.OnQuery(context.Background(), queryWithAttr("prompt", "x"))
	require.ErrorContains(t, err, `filtering on attribute "prompt" is not permitted`)

	allowed := queryWithAttr("service", "checkout")
	got, err := i.OnQuery(context.Background(), allowed)
	require.NoError(t, err)
	assert.Equal(t, allowed, got)
}

func TestOnResult_RedactsConfiguredAttributes(t *testing.T) {
	i := newInterceptor(&Config{RedactAttributes: []string{"prompt"}}, zap.NewNop())

	batch := tracesWithSpanAttrs(map[string]string{"prompt": "my secret", "service": "checkout"})
	out, err := i.OnResult(context.Background(), batch)
	require.NoError(t, err)

	attrs := out[0].ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Attributes()
	prompt, _ := attrs.Get("prompt")
	assert.Equal(t, redactedPlaceholder, prompt.Str())
	service, _ := attrs.Get("service")
	assert.Equal(t, "checkout", service.Str(), "non-redacted attributes are untouched")
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

	// The extension must be usable both as an OTel component and as an interceptor.
	require.NoError(t, ext.Start(context.Background(), componenttest.NewNopHost()))
	require.NoError(t, ext.Shutdown(context.Background()))
	_, ok := ext.(queryinterceptor.Interceptor)
	assert.True(t, ok, "extension must implement queryinterceptor.Interceptor")
}
