// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	depstoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

func rawUITool(t *testing.T, name string) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"name":        name,
		"description": name + " description",
		"parameters":  map[string]any{"type": "object"},
	})
	require.NoError(t, err)
	return b
}

// sharedMCPHandler builds the shared telemetry MCP endpoint the query server owns
// and hands to the gateway — the same server both mounts are served from.
func sharedMCPHandler(t *testing.T) *mcptools.Handler {
	t.Helper()
	return sharedMCPHandlerWithParams(t, mcptools.DefaultServerParams())
}

func sharedMCPHandlerWithParams(t *testing.T, params mcptools.ServerParams) *mcptools.Handler {
	t.Helper()
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	h := mcptools.NewHandler(telemetry.NoopSettings(), svc, tenancy.NewManager(&tenancy.Options{}), params)
	t.Cleanup(func() { h.Close() })
	return h
}

// turnMCPServer mounts a real turn-scoped handler with one active turn
// ("sess-1") holding the given UI tools, and returns the test HTTP server plus
// the recorder backing that turn's SSE stream (to observe UI-tool dispatch).
func turnMCPServer(t *testing.T, uiTools []json.RawMessage) (ts *httptest.Server, rec *httptest.ResponseRecorder, grant turnGrant) {
	t.Helper()
	return turnMCPServerWithParams(t, mcptools.DefaultServerParams(), uiTools)
}

func turnMCPServerWithParams(t *testing.T, params mcptools.ServerParams, uiTools []json.RawMessage) (ts *httptest.Server, rec *httptest.ResponseRecorder, grant turnGrant) {
	t.Helper()
	f := newTurnMCPFixture(t, turnMCPOptions{params: params, uiTools: uiTools})
	return f.ts, f.rec, f.grant
}

// turnMCPOptions configures newTurnMCPFixture. Zero values give the default
// server params and no UI tools.
type turnMCPOptions struct {
	params  mcptools.ServerParams
	uiTools []json.RawMessage
}

// turnMCPFixture is a test server with one active turn, mounted the way the
// query server mounts the gateway: the turn-scoped routes plus the shared
// telemetry mount at /api/ai/mcp/, both served from one MCP server.
type turnMCPFixture struct {
	ts        *httptest.Server
	rec       *httptest.ResponseRecorder
	grant     turnGrant
	closeTurn func()
	turns     *turnRegistry
	// handler is the turn-scoped endpoint's own MCP handler, for tests that
	// observe a session's lifetime behind the route and token checks.
	handler http.Handler
}

func newTurnMCPFixture(t *testing.T, opts turnMCPOptions) turnMCPFixture {
	t.Helper()
	turns := newTurnRegistry()
	rec := httptest.NewRecorder()
	grant, closeTurn := turns.register(newAGUIRun(context.Background(), rec, "thread", "run"), opts.uiTools)

	shared := sharedMCPHandlerWithParams(t, opts.params)
	h := turnScopedEndpointBuilder{
		baseHandler: shared,
		turns:       turns,
		logger:      telemetry.NoopSettings().Logger,
	}.build()
	mux := http.NewServeMux()
	h.registerRoutes(mux)
	mux.Handle(routeMCPPrefix, http.StripPrefix(strings.TrimSuffix(routeMCPPrefix, "/"), shared))

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return turnMCPFixture{
		ts:        ts,
		rec:       rec,
		grant:     grant,
		closeTurn: closeTurn,
		turns:     turns,
		handler:   h.handler,
	}
}

// A session belongs to the turn that opened it. A request on another turn's
// route, with that turn's own token, that names the session is refused, so a
// token holder cannot reach a different turn's session, route id and identity.
func TestTurnSessionIsNotServedOnAnotherTurnsRoute(t *testing.T) {
	f := newTurnMCPFixture(t, turnMCPOptions{})
	other := registerTurn(f.turns, testAGUIRun(), nil)

	session := connectTurnMCP(t, f.ts, routeMCPPrefix+f.grant.routeID+"/", f.grant.token)
	require.NotEmpty(t, session.ID())

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_services"}}`
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, f.ts.URL+routeMCPPrefix+other.routeID+"/", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set(turnTokenHeader, other.token)
	req.Header.Set(mcpSessionIDHeader, session.ID())
	resp, err := f.ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// callToolOnTurnHandler posts one tools/call for sessionID straight to the
// turn-scoped endpoint's handler, past the route and token checks, and returns
// the HTTP status. It observes whether the handler still holds the session:
// 200 while it does, 404 once the session is closed.
func callToolOnTurnHandler(t *testing.T, f turnMCPFixture, sessionID string) int {
	t.Helper()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_services"}}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Session-Id", sessionID)
	rr := httptest.NewRecorder()
	f.handler.ServeHTTP(rr, req)
	return rr.Code
}

// connectTurnMCP connects an MCP client to path. token is the turn's token, sent
// on every request; it is empty for the shared endpoint, which requires none.
func connectTurnMCP(t *testing.T, ts *httptest.Server, path, token string) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	session, err := mcpClient.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   ts.URL + path,
		HTTPClient: turnHTTPClient(ts, token),
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { session.Close() })
	return session
}

// turnHTTPClient returns ts's client, sending token as the turn token header on
// every request when it is not empty.
func turnHTTPClient(ts *httptest.Server, token string) *http.Client {
	if token == "" {
		return ts.Client()
	}
	base := ts.Client()
	return &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		r = r.Clone(r.Context())
		r.Header.Set(turnTokenHeader, token)
		return base.Transport.RoundTrip(r)
	})}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// listToolNames connects an MCP client to path and returns the tool names the
// server advertises there.
func listToolNames(t *testing.T, ts *httptest.Server, path, token string) []string {
	t.Helper()
	session := connectTurnMCP(t, ts, path, token)
	listed, err := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	require.NoError(t, err)
	return toolNames(listed.Tools)
}

func TestTurnScopedEndpointServesTelemetryPlusUITools(t *testing.T) {
	ts, _, grant := turnMCPServer(t, []json.RawMessage{rawUITool(t, "show_chart")})

	got := listToolNames(t, ts, "/api/ai/mcp/"+grant.routeID+"/", grant.token)
	assert.Contains(t, got, "get_services", "built-in telemetry tools must be advertised")
	assert.Contains(t, got, UIToolPrefix+"show_chart", "the turn's UI tools must be advertised under the namespace")
	assert.NotContains(t, got, "show_chart", "the bare frontend name is not an MCP tool")
}

// TestClosingTurnClosesItsMCPSessions pins a session's lifetime to its turn. A
// stateful session created under the turn's route keeps the context of the
// request that created it, identity headers and route id included, for as long
// as the handler holds it, so the turn must close the session when it ends
// rather than leave it to the idle timer. The route answers 404 after the turn
// whether or not the session was closed, so the test asks the endpoint's handler
// directly.
func TestClosingTurnClosesItsMCPSessions(t *testing.T) {
	f := newTurnMCPFixture(t, turnMCPOptions{})

	session := connectTurnMCP(t, f.ts, routeMCPPrefix+f.grant.routeID+"/", f.grant.token)
	_, err := session.ListTools(context.Background(), nil)
	require.NoError(t, err, "the session works while the turn is live")
	require.Equal(t, http.StatusOK, callToolOnTurnHandler(t, f, session.ID()), "the handler holds the turn's session while the turn is live")

	// A second turn on the same handler is the control: closing the first
	// turn must not close its sessions.
	other := registerTurn(f.turns, testAGUIRun(), nil)
	otherSession := connectTurnMCP(t, f.ts, routeMCPPrefix+other.routeID+"/", other.token)

	f.closeTurn()

	// Closing runs on its own goroutine, so the refusal arrives shortly after.
	require.Eventually(t, func() bool {
		return callToolOnTurnHandler(t, f, session.ID()) == http.StatusNotFound
	}, 5*time.Second, 10*time.Millisecond, "a session opened under the turn is closed with it")
	assert.Equal(t, http.StatusOK, callToolOnTurnHandler(t, f, otherSession.ID()), "another turn's session stays open")
}

// TestTurnScopedEndpointStateless checks that the turn's UI tools are still
// advertised when the shared handler runs in stateless mode. ServeHTTP stamps the
// route id on every request, and in stateless mode the SDK builds the session from
// each request's context, so uiToolsMiddleware finds the turn without a session
// that outlives the request. The test talks to the endpoint through the real
// go-sdk client, which after initialize sends a GET to open a stream for
// server-to-client messages; a stateless server answers that GET with 405, and the
// client has to treat that as "no stream" and continue, or ListTools below would
// fail.
func TestTurnScopedEndpointStateless(t *testing.T) {
	params := mcptools.DefaultServerParams()
	params.StreamableHTTP.Stateless = true
	ts, _, grant := turnMCPServerWithParams(t, params, []json.RawMessage{rawUITool(t, "show_chart")})

	got := listToolNames(t, ts, "/api/ai/mcp/"+grant.routeID+"/", grant.token)
	assert.Contains(t, got, "get_services", "built-in telemetry tools must be advertised")
	assert.Contains(t, got, UIToolPrefix+"show_chart", "the turn's UI tools must be advertised under the namespace")
}

// TestSharedMCPHandlerServesTelemetryOnly pins the shared endpoint's contract while a
// chat gateway runs on the same server. Building the gateway installs
// uiToolsMiddleware on that server, so the middleware now sees the external clients'
// requests too; it must resolve no turn for them and leave the tool list alone. The
// turn below is live throughout, so a leak would show up as its UI tool appearing on
// the shared endpoint.
func TestSharedMCPHandlerServesTelemetryOnly(t *testing.T) {
	shared := sharedMCPHandler(t)
	h := NewHandler(HandlerParams{
		Logger:             telemetry.NoopSettings().Logger,
		AgentURL:           "ws://127.0.0.1:1",
		MaxRequestBodySize: 1 << 20,
		MCP:                shared,
	})
	grant := registerTurn(h.mcp.turns, testAGUIRun(), []json.RawMessage{rawUITool(t, "show_chart")})

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	// The shared endpoint, as jaeger-query's mountSharedMCP registers it.
	mux.Handle("/api/ai/mcp/", http.StripPrefix("/api/ai/mcp", shared))
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	require.Contains(t, listToolNames(t, ts, "/api/ai/mcp/"+grant.routeID+"/", grant.token), UIToolPrefix+"show_chart",
		"precondition: the turn's UI tool is live on the turn-scoped endpoint")

	got := listToolNames(t, ts, "/api/ai/mcp/", "")
	assert.Contains(t, got, "get_services", "the shared endpoint serves the telemetry tools")
	assert.NotContains(t, got, UIToolPrefix+"show_chart", "the shared endpoint must never advertise a turn's UI tools")
}

// A turn's MCP session lives on the turn-scoped endpoint's own handler, so its
// session id opens nothing on the shared endpoint, where no turn token is checked.
// Without that, a session id would be a second way into the turn.
func TestTurnSessionIsNotAddressableOnSharedEndpoint(t *testing.T) {
	shared := sharedMCPHandler(t)
	h := NewHandler(HandlerParams{
		Logger:             telemetry.NoopSettings().Logger,
		AgentURL:           "ws://127.0.0.1:1",
		MaxRequestBodySize: 1 << 20,
		MCP:                shared,
	})
	grant := registerTurn(h.mcp.turns, testAGUIRun(), []json.RawMessage{rawUITool(t, "show_chart")})

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.Handle("/api/ai/mcp/", http.StripPrefix("/api/ai/mcp", shared))
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	session := connectTurnMCP(t, ts, "/api/ai/mcp/"+grant.routeID+"/", grant.token)
	require.NotEmpty(t, session.ID(), "precondition: the default transport is stateful and issues a session id")

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, ts.URL+"/api/ai/mcp/",
		strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Session-Id", session.ID())
	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "the shared endpoint does not know the turn's session")
}

// A token outlives nothing: once the turn closes, a request that carries the
// right token is refused like any other request for a dead route id.
func TestTurnTokenIsVoidAfterTurnEnds(t *testing.T) {
	turns := newTurnRegistry()
	grant, closeTurn := turns.register(testAGUIRun(), nil)
	h := turnScopedEndpointBuilder{
		baseHandler: sharedMCPHandler(t),
		turns:       turns,
		logger:      telemetry.NoopSettings().Logger,
	}.build()
	mux := http.NewServeMux()
	h.registerRoutes(mux)

	closeTurn()
	req := httptest.NewRequest(http.MethodGet, "/api/ai/mcp/"+grant.routeID+"/", http.NoBody)
	req.Header.Set(turnTokenHeader, grant.token)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestTurnScopedEndpointDispatchesUIToolToStream(t *testing.T) {
	ts, rec, grant := turnMCPServer(t, []json.RawMessage{rawUITool(t, "show_chart")})
	session := connectTurnMCP(t, ts, "/api/ai/mcp/"+grant.routeID+"/", grant.token)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      UIToolPrefix + "show_chart",
		Arguments: map[string]any{"series": "latency"},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	// The UI-tool call was dispatched to the browser over the turn's SSE
	// stream under the bare name the frontend registered.
	assert.Contains(t, rec.Body.String(), `"toolCallName":"show_chart"`)
	assert.NotContains(t, rec.Body.String(), UIToolPrefix)
}

// TestTurnScopedEndpointIsolatesTurns is the key guarantee of the single
// shared server: two turns declaring different UI tools each see only their
// own (plus the shared telemetry tools), and a UI-tool call reaches only the
// calling turn's stream. If the middleware ever resolved the wrong turn
// from the request context, this would cross the wires.
func TestTurnScopedEndpointIsolatesTurns(t *testing.T) {
	turns := newTurnRegistry()
	recA, recB := httptest.NewRecorder(), httptest.NewRecorder()
	grantA := registerTurn(turns, newAGUIRun(context.Background(), recA, "ta", "ra"), []json.RawMessage{rawUITool(t, "chart_a")})
	grantB := registerTurn(turns, newAGUIRun(context.Background(), recB, "tb", "rb"), []json.RawMessage{rawUITool(t, "chart_b")})

	h := turnScopedEndpointBuilder{
		baseHandler: sharedMCPHandler(t),
		turns:       turns,
		logger:      telemetry.NoopSettings().Logger,
	}.build()
	mux := http.NewServeMux()
	h.registerRoutes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	sessionA := connectTurnMCP(t, ts, "/api/ai/mcp/"+grantA.routeID+"/", grantA.token)
	sessionB := connectTurnMCP(t, ts, "/api/ai/mcp/"+grantB.routeID+"/", grantB.token)

	listA, err := sessionA.ListTools(context.Background(), &mcp.ListToolsParams{})
	require.NoError(t, err)
	listB, err := sessionB.ListTools(context.Background(), &mcp.ListToolsParams{})
	require.NoError(t, err)
	namesA, namesB := toolNames(listA.Tools), toolNames(listB.Tools)

	assert.Contains(t, namesA, UIToolPrefix+"chart_a")
	assert.NotContains(t, namesA, UIToolPrefix+"chart_b", "turn A must not see turn B's UI tools")
	assert.Contains(t, namesB, UIToolPrefix+"chart_b")
	assert.NotContains(t, namesB, UIToolPrefix+"chart_a", "turn B must not see turn A's UI tools")
	assert.Contains(t, namesA, "get_services", "both turns still see the shared telemetry tools")
	assert.Contains(t, namesB, "get_services")

	_, err = sessionA.CallTool(context.Background(), &mcp.CallToolParams{Name: UIToolPrefix + "chart_a"})
	require.NoError(t, err)
	assert.Contains(t, recA.Body.String(), "chart_a", "the dispatch reaches the calling turn's stream")
	assert.NotContains(t, recB.Body.String(), "chart_a", "the other turn's stream is untouched")
}

func TestTurnScopedEndpointRejectsUnknownTurn(t *testing.T) {
	ts, _, _ := turnMCPServer(t, nil)
	for _, p := range []string{"/api/ai/mcp/ghost/mcp", "/api/ai/mcp/ghost"} {
		t.Run(p, func(t *testing.T) {
			resp, err := ts.Client().Get(ts.URL + p)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})
	}
}
