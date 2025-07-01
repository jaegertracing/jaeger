// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
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
		name       string
		tagsConfig escfg.TagsAsFields
	}{
		{
			name: "Empty TagsAsFields with feature enabled",
			tagsConfig: escfg.TagsAsFields{
				Include: "",
			},
		},
		{
			name: "With some tags with feature enabled",
			tagsConfig: escfg.TagsAsFields{
				Include: "foo.bar,baz.qux",
			},
		},
		{
			name: "With one required tag already with feature enabled",
			tagsConfig: escfg.TagsAsFields{
				Include: "span.kind",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

			require.Contains(t, includeTags, model.SpanKindKey)
			require.Contains(t, includeTags, tagError)
		})
	}
}

func TestEnsureRequiredFields_AllAsFieldsTrue(t *testing.T) {
	originalCfg := escfg.Configuration{
		Tags: escfg.TagsAsFields{
			AllAsFields: true,
			Include:     "custom1,custom2,span.kind,error",
		},
	}

	// Make an exact copy for comparison
	expectedCfg := originalCfg
	result := ensureRequiredFields(originalCfg)
	require.Equal(t, expectedCfg, result)
}
