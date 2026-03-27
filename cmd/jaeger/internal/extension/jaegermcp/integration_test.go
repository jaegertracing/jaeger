// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	depstoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
)

// --- Test setup helpers ---

// Shared IDs for integration test trace fixtures.
var (
	testTraceID = pcommon.TraceID([16]byte{
		0xab, 0xcd, 0xef, 0x12, 0x34, 0x56, 0x78, 0x90,
		0xab, 0xcd, 0xef, 0x12, 0x34, 0x56, 0x78, 0x90,
	})
	rootSpanID  = pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1})
	childSpanID = pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 2})
)

// mcpSession bundles a connected MCP client session with a context.
type mcpSession struct {
	*mcp.ClientSession
	ctx context.Context
}

// callTool invokes the named tool, asserts success, and returns the text content.
func (s *mcpSession) callTool(t *testing.T, name string, args map[string]any) string {
	t.Helper()
	result, err := s.CallTool(s.ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)
	return extractTextContent(t, result)
}

// connectMCPSession starts a test server backed by the given mock reader,
// connects an MCP SDK client, and returns the session with a 5s context.
// Pass nil for tests that only exercise protocol-level operations (initialize,
// tools/list, health) which do not hit storage. Passing nil creates a
// QueryService backed by empty mocks that will panic on unexpected storage calls.
func connectMCPSession(t *testing.T, mockReader *tracestoremocks.Reader) *mcpSession {
	t.Helper()
	var svc *querysvc.QueryService
	if mockReader != nil {
		svc = querysvc.NewQueryService(mockReader, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	}
	_, addr := startTestServerWithQueryService(t, svc, nil)
	session := connectMCPClient(t, addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	return &mcpSession{ClientSession: session, ctx: ctx}
}

// connectMCPClient creates an MCP SDK client, connects it to the test server,
// and returns the initialized session. The session is closed via t.Cleanup.
func connectMCPClient(t *testing.T, addr string) *mcp.ClientSession {
	t.Helper()

	client := mcp.NewClient(
		&mcp.Implementation{
			Name:    "jaeger-integration-test",
			Version: "1.0.0",
		},
		nil,
	)

	transport := &mcp.StreamableClientTransport{
		Endpoint:   fmt.Sprintf("http://%s/mcp", addr),
		HTTPClient: http.DefaultClient,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session, err := client.Connect(ctx, transport, nil)
	require.NoError(t, err, "MCP client should connect and initialize")

	t.Cleanup(func() {
		session.Close()
	})

	return session
}

// extractTextContent returns the text from the first TextContent in a result.
func extractTextContent(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, result.Content, "CallToolResult should have content")

	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	t.Fatal("no text content found in CallToolResult")
	return ""
}

// --- Mock reader factories ---

// mockReaderWithTraces returns a mock reader whose GetTraces yields the given trace.
func mockReaderWithTraces(trace ptrace.Traces) *tracestoremocks.Reader {
	r := &tracestoremocks.Reader{}
	r.On("GetTraces", mock.Anything, mock.Anything).Return(
		func(_ context.Context, _ ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{trace}, nil)
			}
		},
	)
	return r
}

// mockReaderWithFindTraces returns a mock reader whose FindTraces yields the given trace.
func mockReaderWithFindTraces(trace ptrace.Traces) *tracestoremocks.Reader {
	r := &tracestoremocks.Reader{}
	r.On("FindTraces", mock.Anything, mock.Anything).Return(
		func(_ context.Context, _ tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{trace}, nil)
			}
		},
	)
	return r
}

// --- Test trace fixtures ---

// newTestTrace builds a two-span trace (root + child). If withError is true
// the child span has an error status; otherwise both spans are OK.
func newTestTrace(withError bool) ptrace.Traces {
	traces := ptrace.NewTraces()
	rs := traces.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "test-service")

	ss := rs.ScopeSpans().AppendEmpty()
	now := time.Now()

	root := ss.Spans().AppendEmpty()
	root.SetTraceID(testTraceID)
	root.SetSpanID(rootSpanID)
	root.SetName("GET /api/traces")
	root.SetStartTimestamp(pcommon.NewTimestampFromTime(now.Add(-100 * time.Millisecond)))
	root.SetEndTimestamp(pcommon.NewTimestampFromTime(now))
	root.Status().SetCode(ptrace.StatusCodeOk)

	child := ss.Spans().AppendEmpty()
	child.SetTraceID(testTraceID)
	child.SetSpanID(childSpanID)
	child.SetParentSpanID(rootSpanID)
	child.SetName("SELECT * FROM traces")
	child.SetStartTimestamp(pcommon.NewTimestampFromTime(now.Add(-80 * time.Millisecond)))
	child.SetEndTimestamp(pcommon.NewTimestampFromTime(now.Add(-10 * time.Millisecond)))
	child.Attributes().PutStr("db.system", "postgresql")

	if withError {
		child.Status().SetCode(ptrace.StatusCodeError)
		child.Status().SetMessage("connection refused")
	} else {
		child.Status().SetCode(ptrace.StatusCodeOk)
	}

	return traces
}

// --- Protocol compliance tests ---

func TestMCPClientInitialize(t *testing.T) {
	s := connectMCPSession(t, nil)
	initResult := s.InitializeResult()
	require.NotNil(t, initResult)
	assert.Equal(t, "jaeger", initResult.ServerInfo.Name)
	assert.Equal(t, "1.0.0", initResult.ServerInfo.Version)
}

func TestMCPClientToolsListDiscovery(t *testing.T) {
	s := connectMCPSession(t, nil)

	result, err := s.ListTools(s.ctx, nil)
	require.NoError(t, err)

	expected := []string{
		"health", "get_services", "get_span_names", "search_traces",
		"get_span_details", "get_trace_errors", "get_trace_topology", "get_critical_path",
	}
	got := make(map[string]bool, len(result.Tools))
	for _, tool := range result.Tools {
		got[tool.Name] = true
	}
	for _, name := range expected {
		assert.True(t, got[name], "tool %q should be discovered via tools/list", name)
	}
	assert.Len(t, result.Tools, len(expected))
}

// --- Tool invocation tests ---

func TestMCPClientHealthTool(t *testing.T) {
	s := connectMCPSession(t, nil)
	text := s.callTool(t, "health", nil)

	var health HealthToolOutput
	require.NoError(t, json.Unmarshal([]byte(text), &health))
	assert.Equal(t, "ok", health.Status)
	assert.Equal(t, "jaeger", health.Server)
	assert.Equal(t, "1.0.0", health.Version)
}

func TestMCPClientGetServices(t *testing.T) {
	mockReader := &tracestoremocks.Reader{}
	mockReader.On("GetServices", mock.Anything).Return(
		[]string{"frontend", "backend", "database"}, nil,
	)
	s := connectMCPSession(t, mockReader)
	text := s.callTool(t, "get_services", nil)

	var output struct {
		Services []string `json:"services"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &output))
	assert.Equal(t, []string{"backend", "database", "frontend"}, output.Services)
}

func TestMCPClientGetSpanNames(t *testing.T) {
	mockReader := &tracestoremocks.Reader{}
	mockReader.On("GetOperations", mock.Anything, mock.Anything).Return(
		[]tracestore.Operation{
			{Name: "GET /api/users", SpanKind: "server"},
			{Name: "POST /api/orders", SpanKind: "server"},
		}, nil,
	)
	s := connectMCPSession(t, mockReader)
	text := s.callTool(t, "get_span_names", map[string]any{"service_name": "frontend"})

	var output struct {
		SpanNames []struct {
			Name     string `json:"name"`
			SpanKind string `json:"span_kind"`
		} `json:"span_names"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &output))
	assert.Len(t, output.SpanNames, 2)
}

func TestMCPClientSearchTraces(t *testing.T) {
	s := connectMCPSession(t, mockReaderWithFindTraces(newTestTrace(false)))
	text := s.callTool(t, "search_traces", map[string]any{
		"service_name":   "test-service",
		"start_time_min": "-1h",
	})
	assert.Contains(t, text, "test-service")
	assert.Contains(t, text, "traces")
}

func TestMCPClientGetTraceTopology(t *testing.T) {
	s := connectMCPSession(t, mockReaderWithTraces(newTestTrace(false)))
	text := s.callTool(t, "get_trace_topology", map[string]any{"trace_id": testTraceID.String()})

	var output struct {
		TraceID string `json:"trace_id"`
		Spans   []any  `json:"spans"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &output))
	assert.Equal(t, testTraceID.String(), output.TraceID)
	assert.NotEmpty(t, output.Spans)
}

func TestMCPClientGetSpanDetails(t *testing.T) {
	s := connectMCPSession(t, mockReaderWithTraces(newTestTrace(false)))
	text := s.callTool(t, "get_span_details", map[string]any{
		"trace_id": testTraceID.String(),
		"span_ids": []string{rootSpanID.String()},
	})

	var output struct {
		TraceID string `json:"trace_id"`
		Spans   []any  `json:"spans"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &output))
	assert.Equal(t, testTraceID.String(), output.TraceID)
	assert.NotEmpty(t, output.Spans)
}

func TestMCPClientGetTraceErrors(t *testing.T) {
	s := connectMCPSession(t, mockReaderWithTraces(newTestTrace(true)))
	text := s.callTool(t, "get_trace_errors", map[string]any{"trace_id": testTraceID.String()})

	var output struct {
		TraceID    string `json:"trace_id"`
		ErrorCount int    `json:"error_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &output))
	assert.Equal(t, testTraceID.String(), output.TraceID)
	assert.Equal(t, 1, output.ErrorCount)
}

func TestMCPClientGetCriticalPath(t *testing.T) {
	s := connectMCPSession(t, mockReaderWithTraces(newTestTrace(false)))
	text := s.callTool(t, "get_critical_path", map[string]any{"trace_id": testTraceID.String()})

	var output struct {
		TraceID  string `json:"trace_id"`
		Segments []any  `json:"segments"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &output))
	assert.Equal(t, testTraceID.String(), output.TraceID)
	assert.NotEmpty(t, output.Segments)
}

// --- End-to-end workflow test ---

// TestMCPClientProgressiveDisclosure exercises the intended LLM interaction
// pattern: services → search → topology → critical path → details.
func TestMCPClientProgressiveDisclosure(t *testing.T) {
	trace := newTestTrace(false)
	tid := testTraceID.String()

	mockReader := mockReaderWithFindTraces(trace)
	mockReader.On("GetServices", mock.Anything).Return([]string{"test-service"}, nil)
	mockReader.On("GetTraces", mock.Anything, mock.Anything).Return(
		func(_ context.Context, _ ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{trace}, nil)
			}
		},
	)

	s := connectMCPSession(t, mockReader)

	// Step 1: Discover services
	assert.Contains(t, s.callTool(t, "get_services", nil), "test-service")

	// Step 2: Search traces
	assert.Contains(t, s.callTool(t, "search_traces", map[string]any{
		"service_name": "test-service", "start_time_min": "-1h",
	}), "test-service")

	// Step 3: Get topology
	assert.Contains(t, s.callTool(t, "get_trace_topology", map[string]any{"trace_id": tid}), "spans")

	// Step 4: Get critical path
	assert.Contains(t, s.callTool(t, "get_critical_path", map[string]any{"trace_id": tid}), "segments")

	// Step 5: Drill into span details
	assert.Contains(t, s.callTool(t, "get_span_details", map[string]any{
		"trace_id": tid, "span_ids": []string{rootSpanID.String()},
	}), "spans")
}

// --- Error handling tests ---

func TestMCPClientInvalidToolCall(t *testing.T) {
	s := connectMCPSession(t, nil)
	_, err := s.CallTool(s.ctx, &mcp.CallToolParams{Name: "nonexistent_tool"})
	assert.Error(t, err)
}

func TestMCPClientSearchTracesMissingRequiredField(t *testing.T) {
	s := connectMCPSession(t, nil)
	_, err := s.CallTool(s.ctx, &mcp.CallToolParams{
		Name:      "search_traces",
		Arguments: map[string]any{"start_time_min": "-1h"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service_name")
}

func TestMCPClientSearchTracesEmptyResults(t *testing.T) {
	mockReader := &tracestoremocks.Reader{}
	mockReader.On("FindTraces", mock.Anything, mock.Anything).Return(
		func(_ context.Context, _ tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(_ func([]ptrace.Traces, error) bool) {}
		},
	)
	s := connectMCPSession(t, mockReader)
	text := s.callTool(t, "search_traces", map[string]any{
		"service_name": "nonexistent", "start_time_min": "-1h",
	})
	assert.NotContains(t, text, `"traces":null`)
	assert.NotContains(t, text, `"traces": null`)
}

// --- Session isolation test ---

func TestMCPClientMultipleSessionsIndependent(t *testing.T) {
	_, addr := startTestServer(t)
	session1 := connectMCPClient(t, addr)
	session2 := connectMCPClient(t, addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	type callResult struct {
		toolResult *mcp.CallToolResult
		err        error
	}
	ch1 := make(chan callResult, 1)
	ch2 := make(chan callResult, 1)

	go func() {
		r, err := session1.CallTool(ctx, &mcp.CallToolParams{Name: "health"})
		ch1 <- callResult{toolResult: r, err: err}
	}()
	go func() {
		r, err := session2.CallTool(ctx, &mcp.CallToolParams{Name: "health"})
		ch2 <- callResult{toolResult: r, err: err}
	}()

	r1 := <-ch1
	r2 := <-ch2

	require.NoError(t, r1.err)
	require.NoError(t, r2.err)
	text1 := extractTextContent(t, r1.toolResult)
	text2 := extractTextContent(t, r2.toolResult)
	assert.Equal(t, text1, text2)
}
