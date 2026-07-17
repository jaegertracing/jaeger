// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package queryinterceptorexample_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/otelcol"

	"github.com/jaegertracing/jaeger/components/exporter/storageexporter"
	"github.com/jaegertracing/jaeger/components/ext/receiver/otlpreceiver"
	"github.com/jaegertracing/jaeger/components/extension/jaegerquery"
	"github.com/jaegertracing/jaeger/components/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/components/extension/queryinterceptorexample"
	"github.com/jaegertracing/jaeger/components/telemetry"
)

// TestExampleConfigValidates loads the shipped config-query-interceptor.yaml
// as-is — no overrides, no hardcoded copy — against the components it references
// and validates it end to end. This proves the file is a correct Jaeger config:
// every referenced component resolves, the service graph validates, and the
// query_interceptor_example extension parses its deny/redact policy from the
// file. The runtime behavior of the two hooks (OnQuery rejecting a forbidden
// filter, OnResult redacting attributes) is covered by extension_test.go, and
// the reader decorator that applies them by reader_decorator_test.go.
func TestExampleConfigValidates(t *testing.T) {
	provider, err := otelcol.NewConfigProvider(otelcol.ConfigProviderSettings{
		ResolverSettings: confmap.ResolverSettings{
			URIs:              []string{"file:config-query-interceptor.yaml"},
			ProviderFactories: []confmap.ProviderFactory{fileprovider.NewFactory(), envprovider.NewFactory()},
		},
	})
	require.NoError(t, err)

	cfg, err := provider.Get(context.Background(), testFactories(t))
	require.NoError(t, err, "config must resolve against the referenced components")
	require.NoError(t, cfg.Validate(), "config must be valid")

	// The example extension parsed its policy straight from the file.
	extCfg, ok := cfg.Extensions[component.MustNewID("query_interceptor_example")].(*queryinterceptorexample.Config)
	require.True(t, ok, "query_interceptor_example extension is not configured in the file")
	assert.Equal(t, "x-jaeger-caller-role", extCfg.IdentityHeader)
	assert.Equal(t, []string{"admin"}, extCfg.PrivilegedRoles)
	assert.Equal(t, []string{"prompt"}, extCfg.DenyQueryAttributes)
	assert.Equal(t, []string{"prompt", "llm.response"}, extCfg.RedactAttributes)
}

// testFactories assembles exactly the public component factories the example
// config references — no internal packages, as a custom OCB build would.
func testFactories(t *testing.T) otelcol.Factories {
	t.Helper()
	f := otelcol.Factories{Telemetry: telemetry.NewFactory()}
	var err error
	f.Extensions, err = otelcol.MakeFactoryMap(
		jaegerstorage.NewFactory(),
		jaegerquery.NewFactory(),
		queryinterceptorexample.NewFactory(),
	)
	require.NoError(t, err)
	f.Receivers, err = otelcol.MakeFactoryMap(otlpreceiver.NewFactory())
	require.NoError(t, err)
	f.Exporters, err = otelcol.MakeFactoryMap(storageexporter.NewFactory())
	require.NoError(t, err)
	return f
}
