// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package queryinterceptorexample_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/otelcol"

	"github.com/jaegertracing/jaeger/components/exporter/storageexporter"
	"github.com/jaegertracing/jaeger/components/ext/receiver/otlpreceiver"
	"github.com/jaegertracing/jaeger/components/extension/jaegerquery"
	"github.com/jaegertracing/jaeger/components/extension/jaegerstorage"
	"github.com/jaegertracing/jaeger/components/extension/queryinterceptorexample"
	"github.com/jaegertracing/jaeger/components/telemetry"
)

const traceID = "00000000000000000000000000abc123"

// TestExampleConfigEndToEnd boots a collector in-process from the shipped
// config-query-interceptor.yaml (only the network endpoints are overridden for
// test isolation), pushes one span via OTLP, and drives the jaeger-query API to
// prove both interceptor hooks the config wires up actually run:
//   - OnResult (return-path) redacts the configured attributes in a fetched trace;
//   - OnQuery (pre-query) rejects a search that filters on a forbidden attribute.
func TestExampleConfigEndToEnd(t *testing.T) {
	// jaeger_query's query service creates a jtracer that OTLP-exports to
	// localhost:4317 by default; disable sampling so it produces nothing to
	// export (same guard as jaegerquery's server_test), avoiding noise and a
	// slow shutdown drain.
	t.Setenv("OTEL_TRACES_SAMPLER", "always_off")

	otlpHTTP := freePort(t)
	queryHTTP := freePort(t)
	// Override only the endpoints; everything else — the query_interceptor_example
	// config and jaeger_query.query_interceptors wiring — comes from the real file.
	override := fmt.Sprintf(`
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 127.0.0.1:%d
      http:
        endpoint: 127.0.0.1:%d
extensions:
  jaeger_query:
    http:
      endpoint: 127.0.0.1:%d
    grpc:
      endpoint: 127.0.0.1:%d
service:
  telemetry:
    metrics:
      level: none
`, freePort(t), otlpHTTP, queryHTTP, freePort(t))

	col, err := otelcol.NewCollector(otelcol.CollectorSettings{
		BuildInfo: component.NewDefaultBuildInfo(),
		Factories: func() (otelcol.Factories, error) { return testFactories(t), nil },
		ConfigProviderSettings: otelcol.ConfigProviderSettings{
			ResolverSettings: confmap.ResolverSettings{
				URIs: []string{"file:config-query-interceptor.yaml", "yaml:" + override},
				ProviderFactories: []confmap.ProviderFactory{
					fileprovider.NewFactory(),
					yamlprovider.NewFactory(),
				},
			},
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- col.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Error("collector did not shut down in time")
		}
	})
	require.Eventually(t, func() bool { return col.GetState() == otelcol.StateRunning },
		20*time.Second, 50*time.Millisecond, "collector did not reach running state")

	pushSpan(t, otlpHTTP)

	// OnResult: the fetched trace has prompt/llm.response redacted, keep intact.
	queryBase := fmt.Sprintf("http://127.0.0.1:%d", queryHTTP)
	var tags map[string]string
	require.Eventually(t, func() bool {
		code, body := httpGet(t, queryBase+"/api/traces/"+traceID)
		if code != http.StatusOK || !strings.Contains(body, `"spans"`) {
			return false
		}
		tags = spanTags(t, body)
		_, ok := tags["prompt"]
		return ok
	}, 10*time.Second, 100*time.Millisecond, "trace not queryable")

	assert.Equal(t, "REDACTED", tags["prompt"], "OnResult should redact prompt")
	assert.Equal(t, "REDACTED", tags["llm.response"], "OnResult should redact llm.response")
	assert.Equal(t, "visible", tags["keep"], "non-configured attribute must be untouched")

	// OnQuery: a search filtering on the forbidden `prompt` attribute is rejected.
	nowUS := time.Now().UnixMicro()
	tagFilter := url.QueryEscape(`{"prompt":"hunter2"}`)
	code, body := httpGet(t, fmt.Sprintf("%s/api/traces?service=checkout&start=%d&end=%d&tags=%s",
		queryBase, nowUS-3600_000_000, nowUS, tagFilter))
	assert.Equal(t, http.StatusInternalServerError, code, "OnQuery should reject the search")
	// The message is JSON-escaped in the response body, so match quote-free substrings.
	assert.Contains(t, body, "query interceptor: filtering on attribute")
	assert.Contains(t, body, "is not permitted")
}

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

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func pushSpan(t *testing.T, otlpHTTPPort int) {
	t.Helper()
	now := time.Now().UnixNano()
	payload := fmt.Sprintf(`{"resourceSpans":[{"resource":{"attributes":[`+
		`{"key":"service.name","value":{"stringValue":"checkout"}}]},"scopeSpans":[{"spans":[{`+
		`"traceId":"%s","spanId":"0000000000abc123","name":"llm-call","kind":2,`+
		`"startTimeUnixNano":"%d","endTimeUnixNano":"%d","attributes":[`+
		`{"key":"prompt","value":{"stringValue":"my password is hunter2"}},`+
		`{"key":"llm.response","value":{"stringValue":"the capital is Paris"}},`+
		`{"key":"keep","value":{"stringValue":"visible"}}]}]}]}]}`, traceID, now, now+1000)

	endpoint := fmt.Sprintf("http://127.0.0.1:%d/v1/traces", otlpHTTPPort)
	var code int
	require.Eventually(t, func() bool {
		resp, err := http.Post(endpoint, "application/json", strings.NewReader(payload))
		if err != nil {
			return false
		}
		resp.Body.Close()
		code = resp.StatusCode
		return true
	}, 10*time.Second, 100*time.Millisecond, "OTLP endpoint not reachable")
	require.Equal(t, http.StatusOK, code, "OTLP push should succeed")
}

func httpGet(t *testing.T, rawURL string) (int, string) {
	t.Helper()
	resp, err := http.Get(rawURL)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

// spanTags extracts the first span's string tags from an /api/traces/{id} response.
func spanTags(t *testing.T, body string) map[string]string {
	t.Helper()
	var resp struct {
		Data []struct {
			Spans []struct {
				Tags []struct {
					Key   string          `json:"key"`
					Value json.RawMessage `json:"value"`
				} `json:"tags"`
			} `json:"spans"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	require.NotEmpty(t, resp.Data)
	require.NotEmpty(t, resp.Data[0].Spans)
	out := map[string]string{}
	for _, tag := range resp.Data[0].Spans[0].Tags {
		var s string
		if json.Unmarshal(tag.Value, &s) == nil {
			out[tag.Key] = s
		}
	}
	return out
}
