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

	acp "github.com/coder/acp-go-sdk"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools"
	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	"github.com/jaegertracing/jaeger/internal/auth/bearertoken"
	"github.com/jaegertracing/jaeger/internal/headerforwarding"
	depstoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
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
	return sharedMCPHandlerWithReader(t, mcptools.DefaultServerParams(), &tracestoremocks.Reader{})
}

func sharedMCPHandlerWithReader(t *testing.T, params mcptools.ServerParams, reader tracestore.Reader) *mcptools.Handler {
	t.Helper()
	svc := querysvc.NewQueryService(reader, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	h := mcptools.NewHandler(telemetry.NoopSettings(), svc, tenancy.NewManager(&tenancy.Options{}), params)
	t.Cleanup(func() { h.Close() })
	return h
}

// turnMCPServer mounts a real turn-scoped handler with one active turn
// ("sess-1") holding the given UI tools, and returns the test HTTP server plus
// the recorder backing that turn's SSE stream (to observe UI-tool dispatch).
func turnMCPServer(t *testing.T, uiTools []json.RawMessage) (ts *httptest.Server, rec *httptest.ResponseRecorder, grant turnGrant) {
	t.Helper()
	f := newTurnMCPFixture(t, turnMCPOptions{uiTools: uiTools})
	return f.ts, f.rec, f.grant
}

// turnMCPOptions configures newTurnMCPFixture. Zero values give no UI tools, a
// reader that answers nothing and no middleware.
type turnMCPOptions struct {
	uiTools []json.RawMessage
	// reader is the trace reader behind the telemetry tools; nil gives a bare
	// mock that answers no call.
	reader tracestore.Reader
	// wrap wraps the fixture's router the way the query server wraps its own,
	// so a test can put the server-wide identity middleware in front of the
	// turn-scoped endpoint. nil installs none.
	wrap func(http.Handler) http.Handler
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
}

func newTurnMCPFixture(t *testing.T, opts turnMCPOptions) turnMCPFixture {
	t.Helper()
	turns := newTurnRegistry()
	rec := httptest.NewRecorder()
	grant, closeTurn := turns.register(newAGUIRun(t.Context(), rec, "thread", "run"), opts.uiTools)

	reader := opts.reader
	if reader == nil {
		reader = &tracestoremocks.Reader{}
	}
	shared := sharedMCPHandlerWithReader(t, mcptools.DefaultServerParams(), reader)
	h := turnScopedEndpointBuilder{
		baseHandler: shared,
		turns:       turns,
		logger:      telemetry.NoopSettings().Logger,
	}.build()
	mux := http.NewServeMux()
	h.registerRoutes(mux)
	mux.Handle(routeMCPPrefix, http.StripPrefix(strings.TrimSuffix(routeMCPPrefix, "/"), shared))

	var handler http.Handler = mux
	if opts.wrap != nil {
		handler = opts.wrap(mux)
	}
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return turnMCPFixture{
		ts:        ts,
		rec:       rec,
		grant:     grant,
		closeTurn: closeTurn,
		turns:     turns,
	}
}

// callerSeen is what a telemetry tool saw in its context: the identity the query
// server's middleware captured from the HTTP request that carried the tool call.
type callerSeen struct {
	bearer string
	user   string
}

// identityFixture mounts the turn-scoped endpoint behind the server-wide bearer
// token and header forwarding middleware, the way the query server mounts it, with
// a trace reader that reports the caller each get_services call ran as.
func identityFixture(t *testing.T) (f turnMCPFixture, forwarded []headerforwarding.ForwardedHeader, calls chan callerSeen) {
	t.Helper()
	forwarded = []headerforwarding.ForwardedHeader{{HTTPName: "X-User", HTTPOutboundName: "X-Forwarded-User"}}
	calls = make(chan callerSeen, 4)
	reader := &tracestoremocks.Reader{}
	reader.On("GetServices", mock.Anything).Run(func(args mock.Arguments) {
		ctx := args.Get(0).(context.Context)
		seen := callerSeen{}
		seen.bearer, _ = bearertoken.GetBearerToken(ctx)
		for _, c := range headerforwarding.CapturedFromContext(ctx) {
			if c.Header.HTTPName == "X-User" {
				seen.user = c.Value
			}
		}
		calls <- seen
	}).Return([]string{"svc"}, nil)
	f = newTurnMCPFixture(t, turnMCPOptions{
		reader: reader,
		wrap: func(next http.Handler) http.Handler {
			return bearertoken.PropagationHandler(zap.NewNop(), headerforwarding.HTTPServerMiddleware(forwarded, next))
		},
	})
	return f, forwarded, calls
}

// announcedHeaders returns the headers the gateway announces for grant when the
// chat request carried the given bearer token and X-User value.
func announcedHeaders(t *testing.T, forwarded []headerforwarding.ForwardedHeader, grant turnGrant, bearer, user string) []acp.HttpHeader {
	t.Helper()
	ctx := bearertoken.ContextWithBearerToken(t.Context(), bearer)
	ctx = headerforwarding.ContextWithCaptured(ctx, []headerforwarding.CapturedHeader{{Header: &forwarded[0], Value: user}})
	return turnServerHeaders(ctx, grant)
}

// The agent sends the announced headers on every request to the turn's MCP
// server, and the query server's own middleware reads them off the tool request,
// so a telemetry tool runs with the chat caller's identity in its context without
// the gateway storing any. A session opened without them runs as nobody.
func TestTurnScopedEndpointRunsToolCallsAsTheAnnouncedCaller(t *testing.T) {
	f, forwarded, calls := identityFixture(t)
	path := routeMCPPrefix + f.grant.routeID + "/"

	withIdentity := connectTurnMCPWithHeaders(t, f.ts, path, announcedHeaders(t, forwarded, f.grant, "tok-alice", "alice"))
	res, err := withIdentity.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_services"})
	require.NoError(t, err)
	require.False(t, res.IsError)
	assert.Equal(t, callerSeen{bearer: "tok-alice", user: "alice"}, <-calls)

	anonymous := connectTurnMCP(t, f.ts, path, f.grant.token)
	res, err = anonymous.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_services"})
	require.NoError(t, err)
	require.False(t, res.IsError)
	assert.Equal(t, callerSeen{}, <-calls, "the token alone carries no identity")
}

// Each tool call runs under the identity of its own HTTP request: the session id
// the endpoint issued to one caller selects neither that caller's identity nor
// their turn when another request carries it. A request on turn B with turn B's
// token and Bob's headers runs as Bob, even when it names the session id Alice's
// client received on turn A, and so does a request on turn A itself with the
// right token.
func TestTurnScopedEndpointRunsEachRequestAsItsOwnCaller(t *testing.T) {
	f, forwarded, calls := identityFixture(t)
	alice := connectTurnMCPWithHeaders(t, f.ts, routeMCPPrefix+f.grant.routeID+"/", announcedHeaders(t, forwarded, f.grant, "tok-alice", "alice"))
	_, err := alice.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_services"})
	require.NoError(t, err)
	require.Equal(t, callerSeen{bearer: "tok-alice", user: "alice"}, <-calls, "precondition: Alice's call runs as Alice")

	grantB, closeB := f.turns.register(newAGUIRun(t.Context(), httptest.NewRecorder(), "thread-b", "run-b"), nil)
	t.Cleanup(closeB)

	callAsBob := func(t *testing.T, grant turnGrant) {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, f.ts.URL+routeMCPPrefix+grant.routeID+"/",
			strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_services","arguments":{}}}`))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Mcp-Session-Id", alice.ID())
		for _, h := range announcedHeaders(t, forwarded, grant, "tok-bob", "bob") {
			req.Header.Set(h.Name, h.Value)
		}
		resp, err := f.ts.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, callerSeen{bearer: "tok-bob", user: "bob"}, <-calls)
	}
	t.Run("on another turn", func(t *testing.T) { callAsBob(t, grantB) })
	t.Run("on the same turn", func(t *testing.T) { callAsBob(t, f.grant) })
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

// connectTurnMCPWithHeaders connects an MCP client to path, sending headers on
// every request the way an agent sends the headers announced in session/new.
func connectTurnMCPWithHeaders(t *testing.T, ts *httptest.Server, path string, headers []acp.HttpHeader) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	session, err := mcpClient.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   ts.URL + path,
		HTTPClient: headersHTTPClient(ts, headers),
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
	return headersHTTPClient(ts, []acp.HttpHeader{{Name: turnTokenHeader, Value: token}})
}

// headersHTTPClient returns ts's client, sending headers on every request.
func headersHTTPClient(ts *httptest.Server, headers []acp.HttpHeader) *http.Client {
	base := ts.Client()
	return &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		r = r.Clone(r.Context())
		for _, h := range headers {
			r.Header.Set(h.Name, h.Value)
		}
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

// The turn-scoped endpoint keeps no session, so the session id its stateless
// handler issues names nothing: the shared endpoint, where no turn token is
// checked, does not know it, so the id is no second way into the turn.
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
	require.NotEmpty(t, session.ID(), "precondition: the SDK issues a session id even to a stateless client")

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
