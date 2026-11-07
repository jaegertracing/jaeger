// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	depstoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

// connectTestClient starts an httptest server around the handler and returns a
// connected MCP client session.
func connectTestClient(t *testing.T, handler http.Handler) *mcp.ClientSession {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   ts.URL,
		HTTPClient: ts.Client(),
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { session.Close() })
	return session
}

// TestNewHandler_ListTools drives the full in-process serving stack (mcp.Server
// + middleware + tenancy + otelhttp) over HTTP and asserts the telemetry tools
// are advertised. ListTools does not touch storage, so the QueryService is
// backed by empty mocks.
func TestNewHandler_ListTools(t *testing.T) {
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	handler := NewHandler(telemetry.NoopSettings(), svc, tenancy.NewManager(&tenancy.Options{}), DefaultServerParams())

	session := connectTestClient(t, handler)
	listed, err := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	require.NoError(t, err)

	got := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		got = append(got, tool.Name)
	}
	assert.ElementsMatch(t, []string{
		"get_services", "get_span_names", "find_traces", "get_span_details", "find_spans",
		"get_trace_errors", "get_trace_topology", "get_critical_path", "get_service_dependencies",
		"read_skill",
	}, got)
}

// TestNewHandler_CallTool exercises a tool end-to-end through the HTTP stack,
// confirming the handler reaches the QueryService and returns a result.
func TestNewHandler_CallTool(t *testing.T) {
	reader := &tracestoremocks.Reader{}
	reader.On("GetServices", mock.Anything).Return([]string{"svc-a", "svc-b"}, nil)
	svc := querysvc.NewQueryService(reader, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	handler := NewHandler(telemetry.NoopSettings(), svc, tenancy.NewManager(&tenancy.Options{}), DefaultServerParams())

	session := connectTestClient(t, handler)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_services"})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	require.NotEmpty(t, result.Content)
	text, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, text.Text, "svc-a")
	assert.Contains(t, text.Text, "svc-b")
}

// TestNewHandler_ToolPanicFailsOnlyThatCall drives a panicking tool through the
// full HTTP stack. The SDK runs tool handlers on goroutines of its own, so an
// unrecovered panic there would terminate the test binary. With recovery in place
// the call returns an error result that tracing records as a failed call, and the
// session keeps serving.
func TestNewHandler_ToolPanicFailsOnlyThatCall(t *testing.T) {
	reader := &tracestoremocks.Reader{}
	reader.On("GetServices", mock.Anything).Run(func(mock.Arguments) { panic("storage failed") }).Once()
	reader.On("GetServices", mock.Anything).Return([]string{"svc-a"}, nil)
	svc := querysvc.NewQueryService(reader, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	capture := newTraceCapture(t)
	telset := telemetry.NoopSettings()
	telset.TracerProvider = capture.provider
	handler := NewHandler(telset, svc, tenancy.NewManager(&tenancy.Options{}), DefaultServerParams())

	session := connectTestClient(t, handler)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_services"})
	require.NoError(t, err)
	assert.True(t, result.IsError)

	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_services"})
	require.NoError(t, err)
	assert.False(t, result.IsError, "the session must keep serving after a panic")

	var toolSpans []tracetest.SpanStub
	for _, span := range capture.exporter.GetSpans() {
		if span.Name == mcpMethodToolsCall+" get_services" {
			toolSpans = append(toolSpans, span)
		}
	}
	require.Len(t, toolSpans, 2)
	assertHasStringAttribute(t, toolSpans[0].Attributes, string(otelsemconv.ErrorType("").Key), errorTypeTool)
}

func TestHandlerAddReceivingMiddleware(t *testing.T) {
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	h := NewHandler(telemetry.NoopSettings(), svc, tenancy.NewManager(&tenancy.Options{}), DefaultServerParams())

	var saw atomic.Bool
	h.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "tools/list" {
				saw.Store(true)
			}
			return next(ctx, method, req)
		}
	})

	session := connectTestClient(t, h)
	_, err := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	require.NoError(t, err)
	assert.True(t, saw.Load(), "middleware added after the handler is built must run for later requests")
}

func TestHandlerAddReceivingMiddlewarePanicFailsOnlyThatCall(t *testing.T) {
	reader := &tracestoremocks.Reader{}
	reader.On("GetServices", mock.Anything).Return([]string{"svc-a"}, nil)
	svc := querysvc.NewQueryService(reader, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	h := NewHandler(telemetry.NoopSettings(), svc, tenancy.NewManager(&tenancy.Options{}), DefaultServerParams())

	h.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if toolNameFromRequest(method, req) == "ui_tool" {
				panic("ui tool failed")
			}
			return next(ctx, method, req)
		}
	})

	session := connectTestClient(t, h)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "ui_tool"})
	require.NoError(t, err)
	assert.True(t, result.IsError)

	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_services"})
	require.NoError(t, err)
	assert.False(t, result.IsError, "the session must keep serving after a panic")
}

// TestNewHandler_LocalhostProtection pins the SDK's DNS-rebinding guard and the
// config switch that turns it off. httptest listens on 127.0.0.1, so a request
// that carries a non-loopback Host header is the shape the guard rejects: the
// same shape a sidecar that dials 127.0.0.1 and preserves Host produces. The
// SDK also honours MCPGODEBUG=disablelocalhostprotection=1, read once at process
// start, so the default subtest fails on a machine with that variable set.
func TestNewHandler_LocalhostProtection(t *testing.T) {
	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`
	post := func(t *testing.T, h http.Handler) *http.Response {
		ts := httptest.NewServer(h)
		t.Cleanup(ts.Close)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL, strings.NewReader(initialize))
		require.NoError(t, err)
		req.Host = "jaeger.example.com"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		resp, err := ts.Client().Do(req)
		require.NoError(t, err)
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	tenancyMgr := tenancy.NewManager(&tenancy.Options{})

	t.Run("default rejects a non-loopback Host over loopback", func(t *testing.T) {
		h := NewHandler(telemetry.NoopSettings(), svc, tenancyMgr, DefaultServerParams())
		resp := post(t, h)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "invalid Host header")
	})

	t.Run("disable_localhost_protection admits it", func(t *testing.T) {
		params := DefaultServerParams()
		params.StreamableHTTP.DisableLocalhostProtection = true
		h := NewHandler(telemetry.NoopSettings(), svc, tenancyMgr, params)
		resp := post(t, h)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

// TestNewHandler_Sessionless pins the request model of MCP revision 2026-07-28 on
// the endpoint: no request depends on an earlier one, so a client behind a load
// balancer without session affinity is served by whichever replica its request
// reaches. Each NewHandler stands in for one jaeger-query process.
func TestNewHandler_Sessionless(t *testing.T) {
	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`
	listTools := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
	post := func(h http.Handler, body, sessionID string) *http.Response {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if sessionID != "" {
			req.Header.Set("Mcp-Session-Id", sessionID)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr.Result()
	}
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	tenancyMgr := tenancy.NewManager(&tenancy.Options{})
	newReplica := func() *Handler {
		return NewHandler(telemetry.NoopSettings(), svc, tenancyMgr, DefaultServerParams())
	}

	t.Run("a legacy initialize is answered without a session id and the other replica serves the next request", func(t *testing.T) {
		first, second := newReplica(), newReplica()
		resp := post(first, initialize, "")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Empty(t, resp.Header.Get("Mcp-Session-Id"), "the endpoint issues no session id")
		resp = post(second, listTools, "")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), `"name":"get_services"`)
	})

	t.Run("a session id sent anyway is ignored", func(t *testing.T) {
		h := newReplica()
		assert.Equal(t, http.StatusOK, post(h, listTools, "not-a-session").StatusCode)
	})

	t.Run("a 2026-07-28 client is served without any handshake", func(t *testing.T) {
		// A current client sends no initialize: every request names its protocol
		// version in the header and in _meta, carries its capabilities in _meta and
		// repeats its method in the Mcp-Method header, and server/discover is how
		// it reads the server's capabilities when it wants them.
		h := newReplica()
		send := func(id int, method string) string {
			body := fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}}}`, id, method)
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			req.Header.Set("MCP-Protocol-Version", "2026-07-28")
			req.Header.Set("Mcp-Method", method)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
			return rr.Body.String()
		}
		body := send(1, "tools/list")
		assert.Contains(t, body, `"name":"get_services"`)
		assert.NotContains(t, body, "unsupported", "the current revision is served, not refused")
		assert.Contains(t, send(2, "server/discover"), `"2026-07-28"`, "discover lists the current revision")
	})

	t.Run("the standing event stream and the session delete are declined", func(t *testing.T) {
		h := newReplica()
		for _, method := range []string{http.MethodGet, http.MethodDelete} {
			req := httptest.NewRequest(method, "/", http.NoBody)
			req.Header.Set("Accept", "text/event-stream")
			req.Header.Set("Mcp-Session-Id", "not-a-session")
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			assert.Equal(t, http.StatusMethodNotAllowed, rr.Code, method)
			assert.Equal(t, "POST", rr.Header().Get("Allow"), method)
		}
	})
}

// TestNewHandler_JSONResponse pins the body form of a single-message POST under
// each setting, and that the go-sdk client, which accepts both forms, completes a
// session against the JSON form.
func TestNewHandler_JSONResponse(t *testing.T) {
	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	tenancyMgr := tenancy.NewManager(&tenancy.Options{})
	contentType := func(t *testing.T, h http.Handler) string {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(initialize))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
		return rr.Header().Get("Content-Type")
	}

	t.Run("default answers with an event stream", func(t *testing.T) {
		h := NewHandler(telemetry.NoopSettings(), svc, tenancyMgr, DefaultServerParams())
		assert.Equal(t, "text/event-stream", contentType(t, h))
	})

	t.Run("json_response answers with a JSON body", func(t *testing.T) {
		params := DefaultServerParams()
		params.StreamableHTTP.JSONResponse = true
		h := NewHandler(telemetry.NoopSettings(), svc, tenancyMgr, params)
		assert.Equal(t, "application/json", contentType(t, h))
	})

	t.Run("go-sdk client completes a session against the JSON form", func(t *testing.T) {
		params := DefaultServerParams()
		params.StreamableHTTP.JSONResponse = true
		h := NewHandler(telemetry.NoopSettings(), svc, tenancyMgr, params)
		ts := httptest.NewServer(h)
		t.Cleanup(ts.Close)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		t.Cleanup(cancel)
		client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
		session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL, HTTPClient: ts.Client()}, nil)
		require.NoError(t, err)
		t.Cleanup(func() { session.Close() })
		listed, err := session.ListTools(ctx, &mcp.ListToolsParams{})
		require.NoError(t, err)
		assert.True(t, slices.ContainsFunc(listed.Tools, func(tool *mcp.Tool) bool { return tool.Name == "get_services" }))
	})
}
