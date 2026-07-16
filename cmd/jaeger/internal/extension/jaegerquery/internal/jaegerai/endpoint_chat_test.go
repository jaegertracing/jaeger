// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	aguitypes "github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/types"
	"github.com/coder/acp-go-sdk"
	"github.com/gorilla/websocket"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
	"github.com/jaegertracing/jaeger/internal/version"
)

type mockACPAgent struct {
	mu sync.Mutex

	initializeReq   *acp.InitializeRequest
	newSessionReq   *acp.NewSessionRequest
	promptReq       *acp.PromptRequest
	closeSessionReq *acp.CloseSessionRequest

	// agentCapabilities is what Initialize advertises back to the gateway.
	// Default zero value (empty caps) matches the legacy sidecar contract;
	// tests that exercise the capability-gated session/close path opt in
	// by setting SessionCapabilities.Close.
	agentCapabilities acp.AgentCapabilities

	// agentInfo is returned as InitializeResponse.AgentInfo when set. Left
	// nil by default to match sidecars that don't advertise identity.
	agentInfo *acp.Implementation

	promptStopReason acp.StopReason

	initializeErr   error
	newSessionErr   error
	promptErr       error
	closeSessionErr error

	// promptHook is called during Prompt before returning, allowing tests to
	// send SessionUpdate notifications via asc.
	promptHook func(context.Context, *acp.AgentSideConnection, acp.PromptRequest)

	// asc is set by startMockACPWebSocketServer so Prompt can send SessionUpdate notifications.
	asc *acp.AgentSideConnection
}

func (*mockACPAgent) Authenticate(context.Context, acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}

func (a *mockACPAgent) Initialize(_ context.Context, params acp.InitializeRequest) (acp.InitializeResponse, error) {
	if a.initializeErr != nil {
		return acp.InitializeResponse{}, a.initializeErr
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	cp := params
	a.initializeReq = &cp
	return acp.InitializeResponse{
		ProtocolVersion:   params.ProtocolVersion,
		AgentCapabilities: a.agentCapabilities,
		AgentInfo:         a.agentInfo,
		AuthMethods:       []acp.AuthMethod{},
	}, nil
}

func (*mockACPAgent) Cancel(context.Context, acp.CancelNotification) error {
	return nil
}

func (a *mockACPAgent) CloseSession(_ context.Context, params acp.CloseSessionRequest) (acp.CloseSessionResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cp := params
	a.closeSessionReq = &cp
	if a.closeSessionErr != nil {
		return acp.CloseSessionResponse{}, a.closeSessionErr
	}
	return acp.CloseSessionResponse{}, nil
}

func (*mockACPAgent) ListSessions(context.Context, acp.ListSessionsRequest) (acp.ListSessionsResponse, error) {
	return acp.ListSessionsResponse{}, nil
}

func (*mockACPAgent) ResumeSession(context.Context, acp.ResumeSessionRequest) (acp.ResumeSessionResponse, error) {
	return acp.ResumeSessionResponse{}, nil
}

func (a *mockACPAgent) NewSession(_ context.Context, params acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	if a.newSessionErr != nil {
		return acp.NewSessionResponse{}, a.newSessionErr
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	cp := params
	a.newSessionReq = &cp
	return acp.NewSessionResponse{SessionId: "sess-test"}, nil
}

func (a *mockACPAgent) Prompt(ctx context.Context, params acp.PromptRequest) (acp.PromptResponse, error) {
	if a.promptErr != nil {
		return acp.PromptResponse{}, a.promptErr
	}
	a.mu.Lock()
	cp := params
	a.promptReq = &cp
	hook := a.promptHook
	conn := a.asc
	a.mu.Unlock()

	if hook != nil && conn != nil {
		hook(ctx, conn, cp)
	}

	reason := a.promptStopReason
	if reason == "" {
		reason = acp.StopReasonEndTurn
	}
	return acp.PromptResponse{StopReason: reason}, nil
}

func (*mockACPAgent) SetSessionConfigOption(context.Context, acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	return acp.SetSessionConfigOptionResponse{}, nil
}

func (*mockACPAgent) SetSessionMode(context.Context, acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	return acp.SetSessionModeResponse{}, nil
}

func (a *mockACPAgent) snapshot() (*acp.InitializeRequest, *acp.NewSessionRequest, *acp.PromptRequest) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.initializeReq, a.newSessionReq, a.promptReq
}

func (a *mockACPAgent) capturedCloseRequest() *acp.CloseSessionRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.closeSessionReq
}

func startMockACPWebSocketServer(t *testing.T, agent *mockACPAgent) (string, func()) {
	t.Helper()

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		adapter := NewWsAdapter(conn, zap.NewNop())
		asc := acp.NewAgentSideConnection(agent, adapter, adapter)
		agent.mu.Lock()
		agent.asc = asc
		agent.mu.Unlock()

		<-asc.Done()
	}))

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		server.Config.Shutdown(ctx)
		server.Close()
	}

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	return wsURL, cleanup
}

// newAGUIRequest builds a minimal AG-UI RunAgentInput carrying a single
// user message with the supplied prompt text. Tests that need richer
// payloads (tools, context, thread/run ids) construct ChatRequest
// inline.
func newAGUIRequest(prompt string) ChatRequest {
	return ChatRequest{
		Messages: []aguitypes.Message{{Role: aguitypes.RoleUser, Content: prompt}},
	}
}

func TestChatEndpointSendsACPProtocolRequests(t *testing.T) {
	agent := &mockACPAgent{}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := newChatEndpoint(zap.NewNop(), nil, newTurnRegistry(), wsURL, "/jaeger", 1<<20)

	reqBody, err := json.Marshal(newAGUIRequest("trace for service checkout"))
	require.NoError(t, err, "failed to marshal request")

	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(reqBody))
	req.Host = "gateway.example:16686"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "unexpected status code, body=%q", rr.Body.String())

	initReq, sessionReq, promptReq := agent.snapshot()
	require.NotNil(t, initReq, "expected initialize request to be captured")
	require.NotNil(t, sessionReq, "expected new session request to be captured")
	require.NotNil(t, promptReq, "expected prompt request to be captured")

	require.EqualValues(t, acp.ProtocolVersionNumber, initReq.ProtocolVersion, "initialize protocol version mismatch")
	require.False(t, initReq.ClientCapabilities.Fs.ReadTextFile || initReq.ClientCapabilities.Fs.WriteTextFile || initReq.ClientCapabilities.Terminal, "unexpected client capabilities in initialize: %+v", initReq.ClientCapabilities)
	require.NotNil(t, initReq.ClientInfo, "client info should not be nil")
	require.Equal(t, "jaeger-ai-gateway", initReq.ClientInfo.Name, "client name mismatch")
	require.Equal(t, version.Get().GitVersion, initReq.ClientInfo.Version, "client version mismatch")

	require.Equal(t, "/", sessionReq.Cwd, "session/new cwd mismatch")
	require.Empty(t, sessionReq.McpServers,
		"the gateway does not announce the turn-scoped MCP endpoint yet — that is a follow-up PR")
	require.Empty(t, sessionReq.Meta,
		"Meta must be omitted when the AG-UI request carries no tools")

	require.EqualValues(t, "sess-test", promptReq.SessionId, "prompt sessionId mismatch")
	require.Len(t, promptReq.Prompt, 1, "prompt content length mismatch")
	require.NotNil(t, promptReq.Prompt[0].Text, "prompt text should not be nil")
	require.Equal(t, "trace for service checkout", promptReq.Prompt[0].Text.Text, "prompt text mismatch")

	require.Equal(t, "text/event-stream", rr.Header().Get("Content-Type"), "content type mismatch")
	require.Equal(t, "no-cache", rr.Header().Get("Cache-Control"), "cache-control mismatch")
	require.Empty(t, rr.Header().Get("Connection"), "Connection is a hop-by-hop header managed by net/http")
}

// spanRecorder wraps an in-memory tracer provider so tests can start a span
// standing in for the otelhttp-created request span, run the handler, then
// inspect what got recorded on it.
type spanRecorder struct {
	provider *tracesdk.TracerProvider
	exporter *tracetest.InMemoryExporter
	span     trace.Span
}

// exportedSpans ends the recorder's span, flushes the provider, and returns
// the exported spans.
func (s *spanRecorder) exportedSpans(t *testing.T) []tracetest.SpanStub {
	t.Helper()
	s.span.End()
	require.NoError(t, s.provider.ForceFlush(context.Background()))
	return s.exporter.GetSpans()
}

// newSpanRecordingRequest builds a request whose context carries a real,
// recording span (as otelhttp middleware would provide in production), so
// ServeHTTP's SpanFromContext(ctx) enriches an actual span instead of a
// no-op one.
func newSpanRecordingRequest(t *testing.T, method, target string, body []byte) (*http.Request, *httptest.ResponseRecorder, *spanRecorder) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := tracesdk.NewTracerProvider(
		tracesdk.WithSyncer(exporter),
		tracesdk.WithSampler(tracesdk.AlwaysSample()),
	)
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	spanCtx, span := provider.Tracer("test").Start(context.Background(), "http.server")
	req := httptest.NewRequest(method, target, bytes.NewReader(body)).WithContext(spanCtx)
	return req, httptest.NewRecorder(), &spanRecorder{provider: provider, exporter: exporter, span: span}
}

func assertHasStringAttribute(t *testing.T, attrs []attribute.KeyValue, key, value string) {
	t.Helper()
	for _, attr := range attrs {
		if string(attr.Key) == key && attr.Value.AsString() == value {
			return
		}
	}
	t.Fatalf("attribute %s=%s not found in %+v", key, value, attrs)
}

func assertLacksAttribute(t *testing.T, attrs []attribute.KeyValue, key string) {
	t.Helper()
	for _, attr := range attrs {
		require.NotEqual(t, key, string(attr.Key), "attribute %s should not be set: %+v", key, attrs)
	}
}

func TestChatEndpointSetsGenAISpanAttributesWithAgentInfo(t *testing.T) {
	agent := &mockACPAgent{
		agentInfo: &acp.Implementation{Name: "gemini-sidecar", Version: "1.2.3"},
	}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := newChatEndpoint(zap.NewNop(), nil, newTurnRegistry(), wsURL, "/jaeger", 1<<20)

	reqBody, err := json.Marshal(newAGUIRequest("trace for service checkout"))
	require.NoError(t, err, "failed to marshal request")

	req, rr, recorder := newSpanRecordingRequest(t, http.MethodPost, "/api/ai/chat", reqBody)
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, "unexpected status code, body=%q", rr.Body.String())

	spans := recorder.exportedSpans(t)
	require.Len(t, spans, 1, "handler must enrich the existing span, not create a new one")
	attrs := spans[0].Attributes

	assertHasStringAttribute(t, attrs, string(otelsemconv.GenAIOperationNameInvokeAgent.Key), "invoke_agent")
	assertHasStringAttribute(t, attrs, string(otelsemconv.GenAIAgentName("").Key), "gemini-sidecar")
	assertHasStringAttribute(t, attrs, string(otelsemconv.GenAIAgentVersion("").Key), "1.2.3")
	assertHasStringAttribute(t, attrs, string(otelsemconv.GenAIConversationID("").Key), "sess-test")
}

func TestChatEndpointSetsGenAISpanAttributesWithoutAgentInfo(t *testing.T) {
	agent := &mockACPAgent{} // legacy sidecar: no AgentInfo advertised
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := newChatEndpoint(zap.NewNop(), nil, newTurnRegistry(), wsURL, "/jaeger", 1<<20)

	reqBody, err := json.Marshal(newAGUIRequest("trace for service checkout"))
	require.NoError(t, err, "failed to marshal request")

	req, rr, recorder := newSpanRecordingRequest(t, http.MethodPost, "/api/ai/chat", reqBody)
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, "unexpected status code, body=%q", rr.Body.String())

	spans := recorder.exportedSpans(t)
	require.Len(t, spans, 1)
	attrs := spans[0].Attributes

	assertHasStringAttribute(t, attrs, string(otelsemconv.GenAIOperationNameInvokeAgent.Key), "invoke_agent")
	assertHasStringAttribute(t, attrs, string(otelsemconv.GenAIConversationID("").Key), "sess-test")
	assertLacksAttribute(t, attrs, string(otelsemconv.GenAIAgentName("").Key))
	assertLacksAttribute(t, attrs, string(otelsemconv.GenAIAgentVersion("").Key))
}

func TestChatEndpointInjectsTraceContextIntoPromptMeta(t *testing.T) {
	agent := &mockACPAgent{}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := newChatEndpoint(zap.NewNop(), nil, newTurnRegistry(), wsURL, "/jaeger", 1<<20)

	reqBody, err := json.Marshal(newAGUIRequest("trace for service checkout"))
	require.NoError(t, err, "failed to marshal request")

	req, rr, recorder := newSpanRecordingRequest(t, http.MethodPost, "/api/ai/chat", reqBody)
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, "unexpected status code, body=%q", rr.Body.String())

	spans := recorder.exportedSpans(t)
	require.Len(t, spans, 1)
	requestSpan := spans[0]

	_, _, promptReq := agent.snapshot()
	require.NotNil(t, promptReq, "expected prompt request to be captured")
	require.NotNil(t, promptReq.Meta, "expected Prompt _meta to carry the injected trace context")

	traceparent, ok := promptReq.Meta["traceparent"].(string)
	require.True(t, ok, "expected traceparent in Prompt _meta, got %+v", promptReq.Meta)
	assert.Contains(t, traceparent, requestSpan.SpanContext.TraceID().String(),
		"injected traceparent should carry the same trace id as the request span, so the sidecar can join the same trace")
}

func TestChatEndpointRegistersTurnInRegistry(t *testing.T) {
	turns := newTurnRegistry()
	var duringTurn int
	agent := &mockACPAgent{
		// Observe the registry mid-turn: by the time Prompt runs, the chat
		// handler has registered this turn's stream.
		promptHook: func(context.Context, *acp.AgentSideConnection, acp.PromptRequest) {
			duringTurn = turns.count()
		},
	}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := newChatEndpoint(zap.NewNop(), nil, newTurnRegistry(), wsURL, "", 1<<20)
	handler.turns = turns

	reqBody, err := json.Marshal(newAGUIRequest("where is the latency"))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(reqBody))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "body=%q", rr.Body.String())
	assert.Equal(t, 1, duringTurn, "the turn's stream must be registered while the turn is in flight")
	assert.Equal(t, 0, turns.count(), "the stream must be removed at end of turn")
}

// TestChatEndpointAnnouncesMCPEndpoint is the end-to-end wiring check: the turn's
// route id is registered in the turn registry and announced to the sidecar as a
// reachable URL, so the agent can finally dial the endpoint that #8973 made serve
// telemetry + UI tools.
func TestChatEndpointAnnouncesMCPEndpoint(t *testing.T) {
	agent := &mockACPAgent{
		agentCapabilities: acp.AgentCapabilities{
			McpCapabilities: acp.McpCapabilities{Http: true},
		},
	}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := newChatEndpoint(zap.NewNop(), nil, newTurnRegistry(), wsURL, "/jaeger", 1<<20)
	handler.mcpServer = mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	handler.mcpBaseURL = "https://jaeger.example.com:16686"

	reqBody, err := json.Marshal(newAGUIRequest("hello"))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(reqBody))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	_, sessionReq, _ := agent.snapshot()
	require.NotNil(t, sessionReq)
	require.Len(t, sessionReq.McpServers, 1, "the endpoint must be announced")
	require.NotNil(t, sessionReq.McpServers[0].Http)

	url := sessionReq.McpServers[0].Http.Url
	assert.True(t, strings.HasPrefix(url, "https://jaeger.example.com:16686/jaeger/api/ai/mcp/"),
		"announced URL must point at this turn's turn-scoped endpoint, got %q", url)
	assert.True(t, strings.HasSuffix(url, "/"))
	assert.Zero(t, handler.turns.count(), "the turn's registry entry is removed once the turn ends")
}

// TestChatEndpointAnnouncesNothingWhenMCPDisabled covers AI chat without MCP: no
// endpoint is mounted, so the sidecar must be pointed at nothing.
func TestChatEndpointAnnouncesNothingWhenMCPDisabled(t *testing.T) {
	agent := &mockACPAgent{
		agentCapabilities: acp.AgentCapabilities{
			McpCapabilities: acp.McpCapabilities{Http: true},
		},
	}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := newChatEndpoint(zap.NewNop(), nil, newTurnRegistry(), wsURL, "", 1<<20) // no mcpServer

	reqBody, err := json.Marshal(newAGUIRequest("hello"))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(reqBody))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	_, sessionReq, _ := agent.snapshot()
	require.NotNil(t, sessionReq)
	assert.Empty(t, sessionReq.McpServers, "no MCP server may be announced when MCP is off")
}

func TestChatEndpointAppendsContextEntriesToPromptBlocks(t *testing.T) {
	agent := &mockACPAgent{}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := newChatEndpoint(zap.NewNop(), nil, newTurnRegistry(), wsURL, "", 1<<20)

	reqBody, err := json.Marshal(ChatRequest{
		Messages: []aguitypes.Message{{Role: aguitypes.RoleUser, Content: "where is the latency?"}},
		Context: []aguitypes.Context{
			{Description: "trace_id", Value: "abc-123"},
			{Value: "user pressed checkout"},
		},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(reqBody))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	_, _, promptReq := agent.snapshot()
	require.NotNil(t, promptReq)
	require.Len(t, promptReq.Prompt, 3, "expected one user-message block plus two context blocks")
	require.Equal(t, "where is the latency?", promptReq.Prompt[0].Text.Text)
	require.Equal(t, "trace_id:\nabc-123", promptReq.Prompt[1].Text.Text)
	require.Equal(t, "user pressed checkout", promptReq.Prompt[2].Text.Text)
}

func TestChatEndpointAttachesContextualToolsToMetaAndStore(t *testing.T) {
	// AG-UI tools on the request should land both on NewSessionRequest._meta
	// (so the sidecar can register them with the LLM) and in
	// ContextualToolsStore (so handleJaegerToolCall can resolve callbacks).
	// Names are UIToolPrefix-prefixed in Meta — the prefix guarantees no
	// collision with built-in MCP tool names — but the store keeps the
	// original *unprefixed* names because handleJaegerToolCall validates
	// the post-strip name (what the frontend registered). The store entry
	// must be cleared once the turn ends.
	agent := &mockACPAgent{}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	store := NewContextualToolsStore()
	handler := newChatEndpoint(zap.NewNop(), store, newTurnRegistry(), wsURL, "", 1<<20)

	reqBody, err := json.Marshal(ChatRequest{
		Messages: []aguitypes.Message{{Role: aguitypes.RoleUser, Content: "hello"}},
		Tools: []aguitypes.Tool{
			{Name: "render_chart", Description: "draw a chart", Parameters: map[string]any{"type": "object"}},
		},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(reqBody))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	_, sessionReq, _ := agent.snapshot()
	require.NotNil(t, sessionReq)
	require.NotEmpty(t, sessionReq.Meta, "Meta must be populated when the request carries tools")
	// Meta is map[string]any, so after JSON round-trip the contextual tools
	// payload arrives as a generic decoded shape rather than typed slices.
	payload, ok := sessionReq.Meta[ContextualToolsMetaKey].(map[string]any)
	require.True(t, ok, "Meta must contain the contextual tools key as a map")
	tools, ok := payload["tools"].([]any)
	require.True(t, ok, "Meta tools entry must decode as a JSON array")
	require.Len(t, tools, 1)
	first, ok := tools[0].(map[string]any)
	require.True(t, ok, "tool entry must decode as an object")
	assert.Equal(t, UIToolPrefix+"render_chart", first["name"],
		"the gateway must prepend UIToolPrefix before exposing the tool")

	// SetForSession was called with the unprefixed snapshot (so dispatch
	// can validate by frontend name), and DeleteForSession must have run
	// via defer once the turn finished.
	assert.Nil(t, store.GetContextualToolsForSession("sess-test"),
		"store entry should be cleared after the turn ends")
}

func TestChatEndpointOmitsMetaWhenNoTools(t *testing.T) {
	agent := &mockACPAgent{}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := newChatEndpoint(zap.NewNop(), NewContextualToolsStore(), newTurnRegistry(), wsURL, "", 1<<20)

	reqBody, err := json.Marshal(newAGUIRequest("hello"))
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(reqBody))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	_, sessionReq, _ := agent.snapshot()
	require.NotNil(t, sessionReq)
	require.Empty(t, sessionReq.Meta,
		"Meta must stay nil/empty when no tools are sent so the sidecar does not see a stale snapshot")
}

func TestChatEndpointEmitsRunFinishedWithStopReasonInResult(t *testing.T) {
	agent := &mockACPAgent{promptStopReason: acp.StopReasonMaxTokens}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := newChatEndpoint(zap.NewNop(), nil, newTurnRegistry(), wsURL, "", 1<<20)
	reqBody, err := json.Marshal(newAGUIRequest("hello"))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(reqBody))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	events := parseSSEEvents(t, rr.Body.String())
	types := eventTypes(events)
	require.Equal(t, []string{"RUN_STARTED", "RUN_FINISHED"}, types,
		"a turn with no streamed text should emit just the lifecycle frames")
	// AG-UI's RUN_FINISHED schema has no top-level stopReason field, so
	// the sidecar's StopReason rides inside the schema-supported `result`
	// payload. The frontend can read result.stopReason if it cares.
	finishedEvent := events[1]
	_, hasStopReasonAtRoot := finishedEvent["stopReason"]
	assert.False(t, hasStopReasonAtRoot,
		"stopReason must not be a top-level field — extras at that level are forbidden by the schema")
	result, ok := finishedEvent["result"].(map[string]any)
	require.True(t, ok, "stopReason must be wrapped in the schema-supported result payload")
	assert.Equal(t, "max_tokens", result["stopReason"])
}

func TestChatEndpointPropagatesThreadAndRunIDs(t *testing.T) {
	agent := &mockACPAgent{}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := newChatEndpoint(zap.NewNop(), nil, newTurnRegistry(), wsURL, "", 1<<20)

	reqBody, err := json.Marshal(ChatRequest{
		ThreadID: "thread-xyz",
		RunID:    "run-789",
		Messages: []aguitypes.Message{{Role: aguitypes.RoleUser, Content: "hello"}},
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(reqBody))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	events := parseSSEEvents(t, rr.Body.String())
	require.NotEmpty(t, events)
	assert.Equal(t, "thread-xyz", events[0]["threadId"])
	assert.Equal(t, "run-789", events[0]["runId"])
}

type failingFlusherResponseWriter struct {
	header http.Header
	status int
}

func (w *failingFlusherResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *failingFlusherResponseWriter) Write([]byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return 0, errors.New("forced write failure")
}

func (w *failingFlusherResponseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
}

func (*failingFlusherResponseWriter) Flush() {}

func TestNewChatEndpointPassesThroughConfig(t *testing.T) {
	store := NewContextualToolsStore()
	h := newChatEndpoint(zap.NewNop(), store, newTurnRegistry(), "ws://localhost:1", "/jaeger", 512)
	require.Equal(t, int64(512), h.maxRequestBodySize, "expected configured maxRequestBodySize")
	require.Equal(t, "ws://localhost:1", h.sidecarWSURL, "expected configured sidecarWSURL")
	require.Equal(t, "/jaeger", h.basePath, "expected configured basePath")
	require.Same(t, store, h.ctxTools, "expected configured ctxTools store")
}

func TestChatEndpointMethodNotAllowed(t *testing.T) {
	handler := newChatEndpoint(zap.NewNop(), nil, newTurnRegistry(), "ws://127.0.0.1:1", "", 1<<20)
	req := httptest.NewRequest(http.MethodGet, "/api/ai/chat", http.NoBody)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusMethodNotAllowed, rr.Code, "unexpected status code")
}

func TestChatEndpointBadRequest(t *testing.T) {
	handler := newChatEndpoint(zap.NewNop(), nil, newTurnRegistry(), "ws://127.0.0.1:1", "", 1<<20)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader("{"))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, "unexpected status code")
}

func TestChatEndpointEmptyPrompt(t *testing.T) {
	handler := newChatEndpoint(zap.NewNop(), nil, newTurnRegistry(), "ws://127.0.0.1:1", "", 1<<20)
	body, err := json.Marshal(newAGUIRequest("   "))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, "unexpected status code")
	require.Contains(t, rr.Body.String(), "messages must include a user message with text content")
}

func TestChatEndpointNoUserMessage(t *testing.T) {
	handler := newChatEndpoint(zap.NewNop(), nil, newTurnRegistry(), "ws://127.0.0.1:1", "", 1<<20)
	body, err := json.Marshal(ChatRequest{
		Messages: []aguitypes.Message{{Role: "assistant", Content: "no user message in this run"}},
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code,
		"a RunAgentInput with no user message must fail with 400, not silently succeed")
	require.Contains(t, rr.Body.String(), "messages must include a user message with text content")
}

func TestChatEndpointRejectsBlankContextualToolName(t *testing.T) {
	// End-to-end check: a tools array carrying a blank name must short-
	// circuit at the gateway boundary with a 400 — the request must
	// never reach the sidecar (no ACP dial) and must never write a
	// poisoned entry into ContextualToolsStore. The unit tests on
	// validateContextualToolNames pin the function-level contract; this
	// test pins that the handler wires the contract into the HTTP path.
	store := NewContextualToolsStore()
	handler := newChatEndpoint(zap.NewNop(), store, newTurnRegistry(), "ws://127.0.0.1:1", "", 1<<20)

	body, err := json.Marshal(ChatRequest{
		Messages: []aguitypes.Message{{Role: aguitypes.RoleUser, Content: "hi"}},
		Tools: []aguitypes.Tool{
			{Name: "good_tool"},
			{Name: "   "}, // blank — would prefix to "ui_   "
		},
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code,
		"a blank tool name must fail validation before any sidecar work happens")
	assert.Contains(t, rr.Body.String(), "tools[1].name",
		"the 400 body must identify the offending tool index so the frontend can fix it")
	assert.Nil(t, store.GetContextualToolsForSession("sess-test"),
		"the store must not have been written — validation runs before SetForSession")
}

func TestChatEndpointRequestBodyTooLarge(t *testing.T) {
	handler := newChatEndpoint(zap.NewNop(), nil, newTurnRegistry(), "ws://127.0.0.1:1", "", 10)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader(`{"messages":[{"role":"user","content":"this body exceeds the 10 byte limit"}]}`))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, rr.Code, "unexpected status code")
}

func TestChatEndpointDialFailure(t *testing.T) {
	handler := newChatEndpoint(zap.NewNop(), nil, newTurnRegistry(), "ws://127.0.0.1:1", "", 1<<20)
	body, err := json.Marshal(newAGUIRequest("hello"))
	require.NoError(t, err, "failed to marshal request")
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadGateway, rr.Code, "unexpected status code")
}

func TestChatEndpointInitializeError(t *testing.T) {
	agent := &mockACPAgent{initializeErr: errors.New("initialize failed")}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := newChatEndpoint(zap.NewNop(), nil, newTurnRegistry(), wsURL, "", 1<<20)
	body, err := json.Marshal(newAGUIRequest("hello"))
	require.NoError(t, err, "failed to marshal request")
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadGateway, rr.Code, "unexpected status code, body=%q", rr.Body.String())
	require.Contains(t, rr.Body.String(), "Error initializing agent", "expected initialize error message")
}

func TestChatEndpointNewSessionError(t *testing.T) {
	agent := &mockACPAgent{newSessionErr: errors.New("new session failed")}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := newChatEndpoint(zap.NewNop(), nil, newTurnRegistry(), wsURL, "", 1<<20)
	body, err := json.Marshal(newAGUIRequest("hello"))
	require.NoError(t, err, "failed to marshal request")
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadGateway, rr.Code, "unexpected status code, body=%q", rr.Body.String())
	require.Contains(t, rr.Body.String(), "Error creating session", "expected session error message")
}

func TestChatEndpointPromptError(t *testing.T) {
	// Once the SSE response has been committed (Content-Type set, RUN_STARTED
	// emitted), a Prompt failure cannot rewrite the status code — instead the
	// handler must emit a RUN_ERROR event so the AG-UI client can finalise.
	agent := &mockACPAgent{promptErr: errors.New("prompt failed")}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := newChatEndpoint(zap.NewNop(), nil, newTurnRegistry(), wsURL, "", 1<<20)
	body, err := json.Marshal(newAGUIRequest("hello"))
	require.NoError(t, err, "failed to marshal request")
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code,
		"SSE response is already committed by the time Prompt is called, so the handler must signal failure inline")
	events := parseSSEEvents(t, rr.Body.String())
	types := eventTypes(events)
	require.Contains(t, types, "RUN_ERROR")
	for _, e := range events {
		if e["type"] == "RUN_ERROR" {
			require.Contains(t, e["message"], "Error starting prompt")
			return
		}
	}
}

func TestChatEndpointSessionUpdateStreamedBeforePromptReturns(t *testing.T) {
	// The agent sends a SessionUpdate notification during Prompt, just before
	// returning the response. acp-go-sdk guarantees all notifications are
	// processed before Prompt() returns (via notificationWg.Wait), so the
	// streamed text must appear in the response without any grace period.
	agent := &mockACPAgent{
		promptHook: func(ctx context.Context, asc *acp.AgentSideConnection, _ acp.PromptRequest) {
			_ = asc.SessionUpdate(ctx, acp.SessionNotification{
				SessionId: "sess-test",
				Update:    acp.UpdateAgentMessageText("streamed-via-notification"),
			})
		},
	}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := newChatEndpoint(zap.NewNop(), nil, newTurnRegistry(), wsURL, "", 1<<20)
	body, err := json.Marshal(newAGUIRequest("hello"))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "streamed-via-notification",
		"streamed delta must be flushed (inside a TEXT_MESSAGE_CONTENT frame) before Prompt() returns")
}

func TestChatEndpointPromptErrorWriteFailure(t *testing.T) {
	// When the response writer fails after headers have been committed, the
	// handler still completes via failRun → write attempts → silent close.
	// The recorder reports whatever status the underlying writer captured;
	// here, Write sets it to 200 on the first call.
	agent := &mockACPAgent{promptErr: errors.New("prompt failed")}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := newChatEndpoint(zap.NewNop(), nil, newTurnRegistry(), wsURL, "", 1<<20)
	body, err := json.Marshal(newAGUIRequest("hello"))
	require.NoError(t, err, "failed to marshal request")
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	w := &failingFlusherResponseWriter{}

	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.status,
		"Write captures status 200 when the first SSE frame attempt fails; no later WriteHeader override is allowed")
}

func TestChatEndpointSessionCloseFiresWhenAgentAdvertisesCapability(t *testing.T) {
	// Agent advertises session/close support; the gateway must issue a
	// session/close RPC for the session it created. The defer runs
	// synchronously inside ServeHTTP (LIFO with adapter.Close), so the
	// captured request is observable as soon as ServeHTTP returns.
	agent := &mockACPAgent{
		agentCapabilities: acp.AgentCapabilities{
			SessionCapabilities: acp.SessionCapabilities{
				Close: &acp.SessionCloseCapabilities{},
			},
		},
	}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := newChatEndpoint(zap.NewNop(), nil, newTurnRegistry(), wsURL, "", 1<<20)
	body, err := json.Marshal(newAGUIRequest("hello"))
	require.NoError(t, err, "failed to marshal request")
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "unexpected status code, body=%q", rr.Body.String())

	closeReq := agent.capturedCloseRequest()
	require.NotNil(t, closeReq, "expected session/close to be issued when agent advertises the capability")
	require.EqualValues(t, "sess-test", closeReq.SessionId, "session/close session id mismatch")
}

func TestChatEndpointSessionCloseErrorIsSwallowed(t *testing.T) {
	// session/close is best-effort cleanup. If the agent returns an error
	// (or the call would otherwise fail), the handler must NOT surface
	// that to the user — the HTTP response has already been streamed by
	// the time the defer runs. This test exercises the error branch of
	// the cleanup hook and asserts the user-visible response is unchanged.
	agent := &mockACPAgent{
		agentCapabilities: acp.AgentCapabilities{
			SessionCapabilities: acp.SessionCapabilities{
				Close: &acp.SessionCloseCapabilities{},
			},
		},
		closeSessionErr: errors.New("close failed"),
	}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := newChatEndpoint(zap.NewNop(), nil, newTurnRegistry(), wsURL, "", 1<<20)
	body, err := json.Marshal(newAGUIRequest("hello"))
	require.NoError(t, err, "failed to marshal request")
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "close failure must not change user-visible status")
	require.NotContains(t, rr.Body.String(), "close failed",
		"close failure must not leak into the response body")

	closeReq := agent.capturedCloseRequest()
	require.NotNil(t, closeReq, "session/close should still have been attempted")
	require.EqualValues(t, "sess-test", closeReq.SessionId, "session/close session id mismatch")
}

func TestChatEndpointSessionCloseSkippedWhenCapabilityAbsent(t *testing.T) {
	// Default agent advertises empty AgentCapabilities, so the gateway
	// MUST NOT issue session/close — older sidecars would return
	// MethodNotFound. This pins down the capability-gate behavior.
	agent := &mockACPAgent{}
	wsURL, cleanup := startMockACPWebSocketServer(t, agent)
	defer cleanup()

	handler := newChatEndpoint(zap.NewNop(), nil, newTurnRegistry(), wsURL, "", 1<<20)
	body, err := json.Marshal(newAGUIRequest("hello"))
	require.NoError(t, err, "failed to marshal request")
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, "unexpected status code, body=%q", rr.Body.String())
	require.Nil(t, agent.capturedCloseRequest(), "session/close must not fire when agent does not advertise the capability")
}
