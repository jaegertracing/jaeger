// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"go.opentelemetry.io/collector/featuregate"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/metrics"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

var mockEsServerResponse = []byte(`
{
	"Version": {
		"Number": "6"
	}
}
`)

func TestNewFactory(t *testing.T) {
	cfg := escfg.Configuration{}
	coreFactory := getTestingFactoryBase(t, &cfg)
	f := &Factory{coreFactory: coreFactory, config: cfg, metricsFactory: metrics.NullFactory}
	_, err := f.CreateTraceReader()
	require.NoError(t, err)
	_, err = f.CreateTraceWriter()
	require.NoError(t, err)
	_, err = f.CreateDependencyReader()
	require.NoError(t, err)
	err = f.Close()
	require.NoError(t, err)
	err = f.Purge(context.Background())
	require.NoError(t, err)
}

func TestESStorageFactoryWithConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(mockEsServerResponse)
	}))
	defer server.Close()
	cfg := escfg.Configuration{
		Servers:  []string{server.URL},
		LogLevel: "error",
	}
	factory, err := NewFactory(context.Background(), cfg, telemetry.NoopSettings())
	require.NoError(t, err)
	defer factory.Close()
}

func TestESStorageFactoryErr(t *testing.T) {
	f, err := NewFactory(context.Background(), escfg.Configuration{}, telemetry.NoopSettings())
	require.ErrorContains(t, err, "failed to create Elasticsearch client: no servers specified")
	require.Nil(t, f)
}

func getTestingFactoryBase(t *testing.T, cfg *escfg.Configuration) *elasticsearch.FactoryBase {
	f := &elasticsearch.FactoryBase{}
	err := elasticsearch.SetFactoryForTest(f, zaptest.NewLogger(t), metrics.NullFactory, cfg)
	require.NoError(t, err)
	return f
}

func TestAlwaysIncludesRequiredTags(t *testing.T) {
	// Set up mock Elasticsearch server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(mockEsServerResponse)
	}))
	defer server.Close()

	tests := []struct {
		name           string
		tagsConfig     escfg.TagsAsFields
		enableFeature  bool
		expectRequired bool
	}{
		{
			name: "Empty TagsAsFields with feature enabled",
			tagsConfig: escfg.TagsAsFields{
				Include: "",
			},
			enableFeature:  true,
			expectRequired: true,
		},
		{
			name: "With some tags with feature enabled",
			tagsConfig: escfg.TagsAsFields{
				Include: "foo.bar,baz.qux",
			},
			enableFeature:  true,
			expectRequired: true,
		},
		{
			name: "With one required tag already with feature enabled",
			tagsConfig: escfg.TagsAsFields{
				Include: "span.kind",
			},
			enableFeature:  true,
			expectRequired: true,
		},
		{
			name: "Feature disabled with AllAsFields true",
			tagsConfig: escfg.TagsAsFields{
				AllAsFields: true,
				Include:     "custom.tag1,custom.tag2",
			},
			enableFeature:  false,
			expectRequired: false,
		},
		{
			name: "Feature disabled with AllAsFields false",
			tagsConfig: escfg.TagsAsFields{
				Include: "custom.tag1,custom.tag2",
			},
			enableFeature:  false,
			expectRequired: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set feature gate state
			err := featuregate.GlobalRegistry().Set("jaeger.es.materializeSpanKindAndStatus", tt.enableFeature)
			require.NoError(t, err)

			cfg := escfg.Configuration{
				Servers:  []string{server.URL},
				LogLevel: "error",
				Tags:     tt.tagsConfig,
			}
			factory, err := NewFactory(context.Background(), cfg, telemetry.NoopSettings())
			require.NoError(t, err)
			defer factory.Close()

			// Verify tag behavior based on test expectations
			includeTags := factory.config.Tags.Include
			if tt.expectRequired {
				require.Contains(t, includeTags, model.SpanKindKey)
				require.Contains(t, includeTags, tagError)
			} else {
				if tt.tagsConfig.AllAsFields {
					require.True(t, factory.config.Tags.AllAsFields)
					require.Equal(t, tt.tagsConfig.Include, includeTags)
				}
				require.NotContains(t, includeTags, model.SpanKindKey)
				require.NotContains(t, includeTags, tagError)
			}
		})
	}
}
