// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/featuregate"
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

func newTestConfig(tags escfg.TagsAsFields, serverURL string) escfg.Configuration {
	return escfg.Configuration{
		Servers:  []string{serverURL},
		LogLevel: "error",
		Tags:     tags,
	}
}

// cleanAndSortTags splits a comma-separated tag string, removes empty tags, and sorts the result.
func cleanAndSortTags(tagString string) []string {
	tags := strings.Split(tagString, ",")
	cleanTags := make([]string, 0, len(tags))
	for _, tag := range tags {
		if tag != "" {
			cleanTags = append(cleanTags, tag)
		}
	}
	sort.Strings(cleanTags)
	return cleanTags
}

func TestEnsureRequiredFields(t *testing.T) {
	// Set up mock Elasticsearch server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(mockEsServerResponse)
	}))
	defer server.Close()

	tests := []struct {
		name         string
		gateEnabled  bool
		tagsConfig   escfg.TagsAsFields
		expectedTags []string
	}{
		{
			name:        "AllAsFields true",
			gateEnabled: true,
			tagsConfig: escfg.TagsAsFields{
				AllAsFields: true,
				Include:     "tag1,tag2",
			},
			expectedTags: []string{"tag1", "tag2"},
		},
		{
			name:        "Gate disabled",
			gateEnabled: false,
			tagsConfig: escfg.TagsAsFields{
				Include: "tag1,tag2",
			},
			expectedTags: []string{"tag1", "tag2"},
		},
		{
			name:        "No required tags",
			gateEnabled: true,
			tagsConfig: escfg.TagsAsFields{
				Include: "tag1, tag2",
			},
			expectedTags: []string{"tag1", "tag2", model.SpanKindKey, tagError},
		},
		{
			name:        "Already have require tag",
			gateEnabled: true,
			tagsConfig: escfg.TagsAsFields{
				Include: "span.kind, error",
			},
			expectedTags: []string{model.SpanKindKey, tagError},
		},
		{
			name:        "Empty include list",
			gateEnabled: true,
			tagsConfig: escfg.TagsAsFields{
				Include: "",
			},
			expectedTags: []string{model.SpanKindKey, tagError},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up feature gate
			err := featuregate.GlobalRegistry().Set("jaeger.es.materializeSpanKindAndStatus", tt.gateEnabled)
			require.NoError(t, err)

			// Create config and run factory
			cfg := newTestConfig(tt.tagsConfig, server.URL)
			factory, err := NewFactory(context.Background(), cfg, telemetry.NoopSettings())
			require.NoError(t, err)
			defer factory.Close()

			// Compare tags
			actualTags := cleanAndSortTags(factory.config.Tags.Include)
			sort.Strings(tt.expectedTags)
			require.Equal(t, tt.expectedTags, actualTags)
		})
	}
}
