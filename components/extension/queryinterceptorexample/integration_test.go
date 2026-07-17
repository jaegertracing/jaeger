// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package queryinterceptorexample_test

import (
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
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/otelcol"
)

const traceID = "00000000000000000000000000abc123"

// TestExampleConfigEndToEnd boots a collector in-process from the shipped
// config-query-interceptor.yaml with no overrides — free ports are injected
// through the same env vars the file already exposes (`${env:...:-default}`),
// so the file itself is what runs. It pushes one span via OTLP and drives the
// jaeger-query API as two callers to prove both hooks fire per caller identity:
//   - OnResult (return-path) redacts the configured attributes for a
//     non-privileged caller but leaves them intact for a privileged one;
//   - OnQuery (pre-query) rejects a search filtering on a forbidden attribute
//     for a non-privileged caller but admits it for a privileged one.
//
// The caller identity travels in a request header and reaches the interceptor
// through the request context — the propagation an access-control extension needs.
func TestExampleConfigEndToEnd(t *testing.T) {
	// jaeger_query's query service creates a jtracer that OTLP-exports to
	// localhost:4317 by default; disable sampling so it produces nothing to
	// export (same guard as jaegerquery's server_test), avoiding noise and a
	// slow shutdown drain.
	t.Setenv("OTEL_TRACES_SAMPLER", "always_off")

	otlpHTTP := freePort(t)
	queryHTTP := freePort(t)
	// Point the file's env-overridable endpoints at free ports.
	t.Setenv("OTLP_GRPC_ENDPOINT", fmt.Sprintf("127.0.0.1:%d", freePort(t)))
	t.Setenv("OTLP_HTTP_ENDPOINT", fmt.Sprintf("127.0.0.1:%d", otlpHTTP))
	t.Setenv("JAEGER_QUERY_HTTP_ENDPOINT", fmt.Sprintf("127.0.0.1:%d", queryHTTP))
	t.Setenv("JAEGER_QUERY_GRPC_ENDPOINT", fmt.Sprintf("127.0.0.1:%d", freePort(t)))

	col, err := otelcol.NewCollector(otelcol.CollectorSettings{
		BuildInfo: component.NewDefaultBuildInfo(),
		Factories: func() (otelcol.Factories, error) { return testFactories(t), nil },
		ConfigProviderSettings: otelcol.ConfigProviderSettings{
			ResolverSettings: confmap.ResolverSettings{
				URIs: []string{"file:config-query-interceptor.yaml"},
				ProviderFactories: []confmap.ProviderFactory{
					fileprovider.NewFactory(),
					envprovider.NewFactory(),
				},
			},
		},
	})
	require.NoError(t, err)

	// t.Context() is canceled just before cleanup functions run, so the
	// collector begins shutting down automatically when the test ends; the
	// cleanup only waits for Run to return.
	done := make(chan error, 1)
	go func() { done <- col.Run(t.Context()) }()
	t.Cleanup(func() {
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			t.Error("collector did not shut down in time")
		}
	})
	require.Eventually(t, func() bool { return col.GetState() == otelcol.StateRunning },
		20*time.Second, 50*time.Millisecond, "collector did not reach running state")

	pushSpan(t, otlpHTTP)
	queryBase := fmt.Sprintf("http://127.0.0.1:%d", queryHTTP)

	// The interceptor decides per caller: the caller's role travels in the
	// X-Jaeger-Caller-Role header and reaches the interceptor through the request
	// context (jaeger_query's http.include_metadata exposes it as OTel client
	// metadata). The config privileges role "admin"; any other caller is
	// restricted. This is the crux of the Option-D PoC: a real extension would
	// resolve the caller against an access-control system instead of a static role.

	// OnResult, privileged caller: sees the sensitive attributes unredacted.
	// This first fetch also serves as the readiness wait for the pushed trace.
	var adminTags map[string]string
	require.Eventually(t, func() bool {
		code, body := httpGet(t, queryBase+"/api/traces/"+traceID, "admin")
		if code != http.StatusOK || !strings.Contains(body, `"spans"`) {
			return false
		}
		adminTags = spanTags(t, body)
		_, ok := adminTags["prompt"]
		return ok
	}, 10*time.Second, 100*time.Millisecond, "trace not queryable")
	assert.Equal(t, "my password is hunter2", adminTags["prompt"], "privileged caller sees prompt unredacted")
	assert.Equal(t, "the capital is Paris", adminTags["llm.response"], "privileged caller sees response unredacted")

	// OnResult, non-privileged caller: same trace, sensitive attributes redacted.
	_, viewerBody := httpGet(t, queryBase+"/api/traces/"+traceID, "viewer")
	viewerTags := spanTags(t, viewerBody)
	assert.Equal(t, "REDACTED", viewerTags["prompt"], "non-privileged caller: prompt redacted")
	assert.Equal(t, "REDACTED", viewerTags["llm.response"], "non-privileged caller: llm.response redacted")
	assert.Equal(t, "visible", viewerTags["keep"], "non-configured attribute must be untouched")

	// OnQuery, per caller: a search filtering on the forbidden `prompt` attribute
	// is admitted for the privileged caller but rejected for everyone else.
	nowUS := time.Now().UnixMicro()
	tagFilter := url.QueryEscape(`{"prompt":"hunter2"}`)
	searchURL := fmt.Sprintf("%s/api/traces?service=checkout&start=%d&end=%d&tags=%s",
		queryBase, nowUS-3600_000_000, nowUS, tagFilter)

	adminCode, _ := httpGet(t, searchURL, "admin")
	assert.Equal(t, http.StatusOK, adminCode, "privileged caller may filter on prompt")

	// The rejection currently surfaces as a 500 (the query path maps all reader
	// errors to Internal Server Error); returning a 4xx uniformly across the
	// v1/api_v3 HTTP and gRPC paths is a separate change.
	viewerCode, viewerSearch := httpGet(t, searchURL, "viewer")
	assert.Equal(t, http.StatusInternalServerError, viewerCode, "non-privileged caller's query is rejected")
	assert.Contains(t, viewerSearch, "filtering on attribute")
	assert.Contains(t, viewerSearch, "is not permitted")
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

// httpGet issues a GET as the given caller role, sent in the identity header
// the example interceptor is configured to read. An empty role sends no header.
func httpGet(t *testing.T, rawURL, role string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, rawURL, http.NoBody)
	require.NoError(t, err)
	if role != "" {
		req.Header.Set("X-Jaeger-Caller-Role", role)
	}
	resp, err := http.DefaultClient.Do(req)
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
