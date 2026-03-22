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

// TestMCPClientInitialize verifies the MCP SDK client can connect, initialize,
// and receive correct server info.
func TestMCPClientInitialize(t *testing.T) {
	_, addr := startTestServer(t)
	session := connectMCPClient(t, addr)

	initResult := session.InitializeResult()
	require.NotNil(t, initResult)
	assert.Equal(t, "jaeger", initResult.ServerInfo.Name)
	assert.Equal(t, "1.0.0", initResult.ServerInfo.Version)
}

// TestMCPClientToolsListDiscovery verifies that tools/list returns all
// registered tools with correct names and descriptions.
func TestMCPClientToolsListDiscovery(t *testing.T) {
	_, addr := startTestServer(t)
	session := connectMCPClient(t, addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	require.NotNil(t, result)

	// The server registers 8 tools
	expectedTools := map[string]bool{
		"health":             false,
		"get_services":       false,
		"get_span_names":     false,
		"search_traces":      false,
		"get_span_details":   false,
		"get_trace_errors":   false,
		"get_trace_topology": false,
		"get_critical_path":  false,
	}

	for _, tool := range result.Tools {
		if _, ok := expectedTools[tool.Name]; ok {
			expectedTools[tool.Name] = true
		}
	}

	for name, found := range expectedTools {
		assert.True(t, found, "tool %q should be discovered via tools/list", name)
	}
	assert.Len(t, result.Tools, len(expectedTools))
}

// TestMCPClientHealthTool verifies the health tool responds correctly
// via the MCP SDK client.
func TestMCPClientHealthTool(t *testing.T) {
	_, addr := startTestServer(t)
	session := connectMCPClient(t, addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "health",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	text := extractTextContent(t, result)
	var health HealthToolOutput
	require.NoError(t, json.Unmarshal([]byte(text), &health))
	assert.Equal(t, "ok", health.Status)
	assert.Equal(t, "jaeger", health.Server)
	assert.Equal(t, "1.0.0", health.Version)
}

// TestMCPClientGetServices tests the get_services tool via MCP SDK client
// with a mock storage backend.
func TestMCPClientGetServices(t *testing.T) {
	mockReader := &tracestoremocks.Reader{}
	mockReader.On("GetServices", mock.Anything).Return(
		[]string{"frontend", "backend", "database"}, nil,
	)

	queryService := querysvc.NewQueryService(mockReader, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	_, addr := startTestServerWithQueryService(t, queryService, nil)
	session := connectMCPClient(t, addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_services",
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	text := extractTextContent(t, result)
	var output struct {
		Services []string `json:"services"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &output))
	assert.Equal(t, []string{"backend", "database", "frontend"}, output.Services)
}

// TestMCPClientGetSpanNames tests the get_span_names tool via MCP SDK client.
func TestMCPClientGetSpanNames(t *testing.T) {
	mockReader := &tracestoremocks.Reader{}
	mockReader.On("GetOperations", mock.Anything, mock.Anything).Return(
		[]tracestore.Operation{
			{Name: "GET /api/users", SpanKind: "server"},
			{Name: "POST /api/orders", SpanKind: "server"},
		}, nil,
	)

	queryService := querysvc.NewQueryService(mockReader, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	_, addr := startTestServerWithQueryService(t, queryService, nil)
	session := connectMCPClient(t, addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_span_names",
		Arguments: map[string]any{
			"service_name": "frontend",
		},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	text := extractTextContent(t, result)
	var output struct {
		SpanNames []struct {
			Name     string `json:"name"`
			SpanKind string `json:"span_kind"`
		} `json:"span_names"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &output))
	assert.Len(t, output.SpanNames, 2)
}

// TestMCPClientSearchTraces tests the search_traces tool via MCP SDK client
// with mocked trace data.
func TestMCPClientSearchTraces(t *testing.T) {
	testTrace := createMultiSpanTestTrace()

	mockReader := &tracestoremocks.Reader{}
	mockReader.On("FindTraces", mock.Anything, mock.Anything).Return(
		func(_ context.Context, _ tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace}, nil)
			}
		},
	)

	queryService := querysvc.NewQueryService(mockReader, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	_, addr := startTestServerWithQueryService(t, queryService, nil)
	session := connectMCPClient(t, addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "search_traces",
		Arguments: map[string]any{
			"service_name":   "test-service",
			"start_time_min": "-1h",
		},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	text := extractTextContent(t, result)
	assert.Contains(t, text, "test-service")
	assert.Contains(t, text, "traces")
}

// TestMCPClientGetTraceTopology tests the get_trace_topology tool via MCP SDK client.
func TestMCPClientGetTraceTopology(t *testing.T) {
	testTrace := createMultiSpanTestTrace()
	traceID := extractTraceID(testTrace)

	mockReader := &tracestoremocks.Reader{}
	mockReader.On("GetTraces", mock.Anything, mock.Anything).Return(
		func(_ context.Context, _ ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace}, nil)
			}
		},
	)

	queryService := querysvc.NewQueryService(mockReader, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	_, addr := startTestServerWithQueryService(t, queryService, nil)
	session := connectMCPClient(t, addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_trace_topology",
		Arguments: map[string]any{
			"trace_id": traceID,
		},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	text := extractTextContent(t, result)
	var output struct {
		TraceID string `json:"trace_id"`
		Spans   []struct {
			Path     string `json:"path"`
			Service  string `json:"service"`
			SpanName string `json:"span_name"`
		} `json:"spans"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &output))
	assert.Equal(t, traceID, output.TraceID)
	assert.NotEmpty(t, output.Spans)
}

// TestMCPClientGetSpanDetails tests the get_span_details tool via MCP SDK client.
func TestMCPClientGetSpanDetails(t *testing.T) {
	testTrace := createMultiSpanTestTrace()
	traceID := extractTraceID(testTrace)
	spanID := extractFirstSpanID(testTrace)

	mockReader := &tracestoremocks.Reader{}
	mockReader.On("GetTraces", mock.Anything, mock.Anything).Return(
		func(_ context.Context, _ ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace}, nil)
			}
		},
	)

	queryService := querysvc.NewQueryService(mockReader, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	_, addr := startTestServerWithQueryService(t, queryService, nil)
	session := connectMCPClient(t, addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_span_details",
		Arguments: map[string]any{
			"trace_id": traceID,
			"span_ids": []string{spanID},
		},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	text := extractTextContent(t, result)
	var output struct {
		TraceID string `json:"trace_id"`
		Spans   []struct {
			SpanID  string `json:"span_id"`
			Service string `json:"service"`
		} `json:"spans"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &output))
	assert.Equal(t, traceID, output.TraceID)
	assert.NotEmpty(t, output.Spans)
}

// TestMCPClientGetTraceErrors tests the get_trace_errors tool via MCP SDK client.
func TestMCPClientGetTraceErrors(t *testing.T) {
	testTrace := createTestTraceWithErrors()
	traceID := extractTraceID(testTrace)

	mockReader := &tracestoremocks.Reader{}
	mockReader.On("GetTraces", mock.Anything, mock.Anything).Return(
		func(_ context.Context, _ ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace}, nil)
			}
		},
	)

	queryService := querysvc.NewQueryService(mockReader, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	_, addr := startTestServerWithQueryService(t, queryService, nil)
	session := connectMCPClient(t, addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_trace_errors",
		Arguments: map[string]any{
			"trace_id": traceID,
		},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	text := extractTextContent(t, result)
	var output struct {
		TraceID    string `json:"trace_id"`
		ErrorCount int    `json:"error_count"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &output))
	assert.Equal(t, traceID, output.TraceID)
	assert.Equal(t, 1, output.ErrorCount)
}

// TestMCPClientGetCriticalPath tests the get_critical_path tool via MCP SDK client.
func TestMCPClientGetCriticalPath(t *testing.T) {
	testTrace := createMultiSpanTestTrace()
	traceID := extractTraceID(testTrace)

	mockReader := &tracestoremocks.Reader{}
	mockReader.On("GetTraces", mock.Anything, mock.Anything).Return(
		func(_ context.Context, _ ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace}, nil)
			}
		},
	)

	queryService := querysvc.NewQueryService(mockReader, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	_, addr := startTestServerWithQueryService(t, queryService, nil)
	session := connectMCPClient(t, addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_critical_path",
		Arguments: map[string]any{
			"trace_id": traceID,
		},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	text := extractTextContent(t, result)
	var output struct {
		TraceID  string `json:"trace_id"`
		Segments []struct {
			SpanID   string `json:"span_id"`
			Service  string `json:"service"`
			SpanName string `json:"span_name"`
		} `json:"segments"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &output))
	assert.Equal(t, traceID, output.TraceID)
	assert.NotEmpty(t, output.Segments)
}

// TestMCPClientProgressiveDisclosure tests the progressive disclosure workflow:
// search → topology → critical path → details, simulating how an LLM would use the tools.
func TestMCPClientProgressiveDisclosure(t *testing.T) {
	testTrace := createMultiSpanTestTrace()
	traceID := extractTraceID(testTrace)

	mockReader := &tracestoremocks.Reader{}
	mockReader.On("GetServices", mock.Anything).Return(
		[]string{"test-service"}, nil,
	)
	mockReader.On("FindTraces", mock.Anything, mock.Anything).Return(
		func(_ context.Context, _ tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace}, nil)
			}
		},
	)
	mockReader.On("GetTraces", mock.Anything, mock.Anything).Return(
		func(_ context.Context, _ ...tracestore.GetTraceParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(yield func([]ptrace.Traces, error) bool) {
				yield([]ptrace.Traces{testTrace}, nil)
			}
		},
	)

	queryService := querysvc.NewQueryService(mockReader, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	_, addr := startTestServerWithQueryService(t, queryService, nil)
	session := connectMCPClient(t, addr)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Step 1: Discover services
	servicesResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_services",
	})
	require.NoError(t, err)
	assert.False(t, servicesResult.IsError)
	servicesText := extractTextContent(t, servicesResult)
	assert.Contains(t, servicesText, "test-service")

	// Step 2: Search traces for the discovered service
	searchResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "search_traces",
		Arguments: map[string]any{
			"service_name":   "test-service",
			"start_time_min": "-1h",
		},
	})
	require.NoError(t, err)
	assert.False(t, searchResult.IsError)
	searchText := extractTextContent(t, searchResult)
	assert.Contains(t, searchText, "test-service")

	// Step 3: Get topology for the found trace
	topologyResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_trace_topology",
		Arguments: map[string]any{
			"trace_id": traceID,
		},
	})
	require.NoError(t, err)
	assert.False(t, topologyResult.IsError)
	topologyText := extractTextContent(t, topologyResult)
	assert.Contains(t, topologyText, "spans")

	// Step 4: Get critical path
	critPathResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_critical_path",
		Arguments: map[string]any{
			"trace_id": traceID,
		},
	})
	require.NoError(t, err)
	assert.False(t, critPathResult.IsError)
	critPathText := extractTextContent(t, critPathResult)
	assert.Contains(t, critPathText, "segments")

	// Step 5: Drill into span details
	spanID := extractFirstSpanID(testTrace)
	detailsResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_span_details",
		Arguments: map[string]any{
			"trace_id": traceID,
			"span_ids": []string{spanID},
		},
	})
	require.NoError(t, err)
	assert.False(t, detailsResult.IsError)
	detailsText := extractTextContent(t, detailsResult)
	assert.Contains(t, detailsText, "spans")
}

// TestMCPClientInvalidToolCall tests error handling for invalid tool invocations.
func TestMCPClientInvalidToolCall(t *testing.T) {
	_, addr := startTestServer(t)
	session := connectMCPClient(t, addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Call a non-existent tool — the SDK should return an error
	_, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "nonexistent_tool",
	})
	assert.Error(t, err)
}

// TestMCPClientSearchTracesMissingRequiredField tests that search_traces
// returns an error when required fields are missing.
func TestMCPClientSearchTracesMissingRequiredField(t *testing.T) {
	_, addr := startTestServer(t)
	session := connectMCPClient(t, addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Call search_traces without service_name (required field)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "search_traces",
		Arguments: map[string]any{
			"start_time_min": "-1h",
		},
	})
	// The handler may return an error in IsError or the SDK may propagate it
	if err == nil {
		assert.True(t, result.IsError, "search_traces without service_name should return error")
	}
}

// TestMCPClientSearchTracesEmptyResults verifies that search with no matching
// traces returns properly (no null values).
func TestMCPClientSearchTracesEmptyResults(t *testing.T) {
	mockReader := &tracestoremocks.Reader{}
	mockReader.On("FindTraces", mock.Anything, mock.Anything).Return(
		func(_ context.Context, _ tracestore.TraceQueryParams) iter.Seq2[[]ptrace.Traces, error] {
			return func(_ func([]ptrace.Traces, error) bool) {
				// yield nothing
			}
		},
	)

	queryService := querysvc.NewQueryService(mockReader, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	_, addr := startTestServerWithQueryService(t, queryService, nil)
	session := connectMCPClient(t, addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "search_traces",
		Arguments: map[string]any{
			"service_name":   "nonexistent",
			"start_time_min": "-1h",
		},
	})
	require.NoError(t, err)
	assert.False(t, result.IsError)

	text := extractTextContent(t, result)
	assert.NotContains(t, text, `"traces":null`)
	assert.NotContains(t, text, `"traces": null`)
}

// TestMCPClientMultipleSessionsIndependent verifies that two separate
// MCP sessions can coexist and return correct results independently.
func TestMCPClientMultipleSessionsIndependent(t *testing.T) {
	_, addr := startTestServer(t)
	session1 := connectMCPClient(t, addr)
	session2 := connectMCPClient(t, addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	type result struct {
		toolResult *mcp.CallToolResult
		err        error
	}
	ch1 := make(chan result, 1)
	ch2 := make(chan result, 1)

	// Run both calls concurrently to validate session isolation.
	go func() {
		r, err := session1.CallTool(ctx, &mcp.CallToolParams{Name: "health"})
		ch1 <- result{toolResult: r, err: err}
	}()
	go func() {
		r, err := session2.CallTool(ctx, &mcp.CallToolParams{Name: "health"})
		ch2 <- result{toolResult: r, err: err}
	}()

	r1 := <-ch1
	r2 := <-ch2

	require.NoError(t, r1.err)
	require.NoError(t, r2.err)
	// Extract text on the test goroutine (not in the spawned goroutines)
	// to satisfy testifylint's go-require rule.
	text1 := extractTextContent(t, r1.toolResult)
	text2 := extractTextContent(t, r2.toolResult)
	assert.Equal(t, text1, text2)
}

// --- Helpers ---

// extractTextContent extracts the text from the first TextContent in a CallToolResult.
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

// createMultiSpanTestTrace creates a trace with parent-child spans for integration tests.
func createMultiSpanTestTrace() ptrace.Traces {
	traces := ptrace.NewTraces()
	resourceSpans := traces.ResourceSpans().AppendEmpty()
	resourceSpans.Resource().Attributes().PutStr("service.name", "test-service")

	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()
	now := time.Now()

	tid := pcommon.TraceID([16]byte{
		0xab, 0xcd, 0xef, 0x12, 0x34, 0x56, 0x78, 0x90,
		0xab, 0xcd, 0xef, 0x12, 0x34, 0x56, 0x78, 0x90,
	})

	// Root span
	rootSpan := scopeSpans.Spans().AppendEmpty()
	rootSpan.SetTraceID(tid)
	rootSpan.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1}))
	rootSpan.SetName("GET /api/traces")
	rootSpan.SetStartTimestamp(pcommon.NewTimestampFromTime(now.Add(-100 * time.Millisecond)))
	rootSpan.SetEndTimestamp(pcommon.NewTimestampFromTime(now))
	rootSpan.Status().SetCode(ptrace.StatusCodeOk)

	// Child span
	childSpan := scopeSpans.Spans().AppendEmpty()
	childSpan.SetTraceID(tid)
	childSpan.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 2}))
	childSpan.SetParentSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1}))
	childSpan.SetName("SELECT * FROM traces")
	childSpan.SetStartTimestamp(pcommon.NewTimestampFromTime(now.Add(-80 * time.Millisecond)))
	childSpan.SetEndTimestamp(pcommon.NewTimestampFromTime(now.Add(-10 * time.Millisecond)))
	childSpan.Status().SetCode(ptrace.StatusCodeOk)
	childSpan.Attributes().PutStr("db.system", "postgresql")

	return traces
}

// createTestTraceWithErrors creates a trace that has one span with an error status.
func createTestTraceWithErrors() ptrace.Traces {
	traces := ptrace.NewTraces()
	resourceSpans := traces.ResourceSpans().AppendEmpty()
	resourceSpans.Resource().Attributes().PutStr("service.name", "test-service")

	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()
	now := time.Now()

	tid := pcommon.TraceID([16]byte{
		0xab, 0xcd, 0xef, 0x12, 0x34, 0x56, 0x78, 0x90,
		0xab, 0xcd, 0xef, 0x12, 0x34, 0x56, 0x78, 0x90,
	})

	// Root span (ok)
	rootSpan := scopeSpans.Spans().AppendEmpty()
	rootSpan.SetTraceID(tid)
	rootSpan.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1}))
	rootSpan.SetName("GET /api/traces")
	rootSpan.SetStartTimestamp(pcommon.NewTimestampFromTime(now.Add(-100 * time.Millisecond)))
	rootSpan.SetEndTimestamp(pcommon.NewTimestampFromTime(now))
	rootSpan.Status().SetCode(ptrace.StatusCodeOk)

	// Error span
	errorSpan := scopeSpans.Spans().AppendEmpty()
	errorSpan.SetTraceID(tid)
	errorSpan.SetSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 2}))
	errorSpan.SetParentSpanID(pcommon.SpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 1}))
	errorSpan.SetName("SELECT * FROM traces")
	errorSpan.SetStartTimestamp(pcommon.NewTimestampFromTime(now.Add(-80 * time.Millisecond)))
	errorSpan.SetEndTimestamp(pcommon.NewTimestampFromTime(now.Add(-10 * time.Millisecond)))
	errorSpan.Status().SetCode(ptrace.StatusCodeError)
	errorSpan.Status().SetMessage("connection refused")

	return traces
}

// extractTraceID extracts the hex trace ID string from a ptrace.Traces object.
func extractTraceID(traces ptrace.Traces) string {
	return traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).TraceID().String()
}

// extractFirstSpanID extracts the hex span ID of the first span.
func extractFirstSpanID(traces ptrace.Traces) string {
	return traces.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).SpanID().String()
}
