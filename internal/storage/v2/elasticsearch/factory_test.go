// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

var mockEsServerResponse = []byte(`
{
	"version": {
		"number": "7.10.2"
	},
	"tagline": "You Know, for Search"
}
`)

// func TestNewFactory(t *testing.T) {
// 	cfg := escfg.Configuration{}
// 	coreFactory := getTestingFactoryBase(t, &cfg)
// 	f := &Factory{coreFactory: coreFactory, config: cfg, metricsFactory: metrics.NullFactory}
// 	_, err := f.CreateTraceReader()
// 	require.NoError(t, err)
// 	_, err = f.CreateTraceWriter()
// 	require.NoError(t, err)
// 	_, err = f.CreateDependencyReader()
// 	require.NoError(t, err)
// 	_, err = f.CreateSamplingStore(1)
// 	require.NoError(t, err)
// 	err = f.Close()
// 	require.NoError(t, err)
// 	err = f.Purge(context.Background())
// 	require.NoError(t, err)
// }

func TestESStorageFactoryWithConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(mockEsServerResponse)
	}))
	defer server.Close()
	cfg := escfg.Configuration{
		Servers:  []string{server.URL},
		LogLevel: "error",
	}
	factory, err := NewFactory(context.Background(), cfg, telemetry.NoopSettings(), nil)
	require.NoError(t, err)
	factory.Close()
}

// TestSyncWriteModePropagatesBulkError asserts the RFC 0007 M4 wiring end-to-end:
// with write_mode: sync, a failing _bulk request surfaces as a WriteTraces error
// (unlike async mode, which returns nil at enqueue time). The mock backend answers
// the version probe but rejects _bulk with 500.
func TestSyncWriteModePropagatesBulkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "_bulk") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write(mockEsServerResponse)
	}))
	defer server.Close()

	cfg := escfg.Configuration{
		Servers:   []string{server.URL},
		WriteMode: escfg.WriteModeSync,
		LogLevel:  "error",
	}
	factory, err := NewFactory(context.Background(), cfg, telemetry.NoopSettings(), nil)
	require.NoError(t, err)
	defer factory.Close()

	writer, err := factory.CreateTraceWriter()
	require.NoError(t, err)

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "svc")
	span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetName("op")
	span.SetTraceID(pcommon.TraceID([16]byte{1}))
	span.SetSpanID(pcommon.SpanID([8]byte{2}))

	err = writer.WriteTraces(context.Background(), td)
	require.Error(t, err, "a failing _bulk must surface as a WriteTraces error in sync mode")
}

func TestESStorageFactoryErr(t *testing.T) {
	f, err := NewFactory(context.Background(), escfg.Configuration{}, telemetry.NoopSettings(), nil)
	require.ErrorContains(t, err, "no servers specified")
	require.Nil(t, f)
}

// func getTestingFactoryBase(t *testing.T, cfg *escfg.Configuration) *elasticsearch.FactoryBase {
// 	f := &elasticsearch.FactoryBase{}
// 	err := elasticsearch.SetFactoryForTest(f, zaptest.NewLogger(t), metrics.NullFactory, cfg)
// 	require.NoError(t, err)
// 	return f
// }

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
			factory, err := NewFactory(context.Background(), cfg, telemetry.NoopSettings(), nil)
			require.NoError(t, err)
			defer factory.Close()

			// Verify tag behavior based on test expectations
			includeTags := factory.config.Tags.Include

			require.Contains(t, includeTags, model.SpanKindKey)
			require.Contains(t, includeTags, tagError)
		})
	}
}

func TestCreateTraceReader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(mockEsServerResponse)
	}))
	defer server.Close()

	cfg := escfg.Configuration{Servers: []string{server.URL}, LogLevel: "error"}
	factory, err := NewFactory(context.Background(), cfg, telemetry.NoopSettings(), nil)
	require.NoError(t, err)
	defer factory.Close()

	// Gate-driven behavior of FindTraceSummaries is covered by the tracestore package
	// unit tests; here we only assert the factory wires up a usable reader.
	reader, err := factory.CreateTraceReader()
	require.NoError(t, err)
	require.NotNil(t, reader)
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
