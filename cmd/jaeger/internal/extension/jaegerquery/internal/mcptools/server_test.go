// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	depstoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
	"github.com/jaegertracing/jaeger/internal/telemetry"
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
	handler, err := NewHandler(telemetry.NoopSettings(), svc, tenancy.NewManager(&tenancy.Options{}), DefaultConfig())
	require.NoError(t, err)

	session := connectTestClient(t, handler)
	listed, err := session.ListTools(context.Background(), &mcp.ListToolsParams{})
	require.NoError(t, err)

	got := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		got = append(got, tool.Name)
	}
	assert.ElementsMatch(t, []string{
		"get_services", "get_span_names", "search_traces", "get_span_details",
		"get_trace_errors", "get_trace_topology", "get_critical_path", "get_service_dependencies",
		"read_skill",
	}, got)

	// Drift guard: registeredToolNames (used to validate operator skills'
	// allowed-tools lists) must name exactly the tools actually advertised.
	registeredToolNames, err := registerTools(mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil), svc, DefaultConfig(), zap.NewNop())
	require.NoError(t, err)
	gotSet := make(map[string]bool, len(got))
	for _, name := range got {
		gotSet[name] = true
	}
	assert.Equal(t, gotSet, registeredToolNames)
}

// TestNewHandler_CallTool exercises a tool end-to-end through the HTTP stack,
// confirming the handler reaches the QueryService and returns a result.
func TestNewHandler_CallTool(t *testing.T) {
	reader := &tracestoremocks.Reader{}
	reader.On("GetServices", mock.Anything).Return([]string{"svc-a", "svc-b"}, nil)
	svc := querysvc.NewQueryService(reader, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	handler, err := NewHandler(telemetry.NoopSettings(), svc, tenancy.NewManager(&tenancy.Options{}), DefaultConfig())
	require.NoError(t, err)

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

// TestNewServerDegradesWithoutMetrics covers the branch where the metrics
// middleware fails to build: the server is still returned (metrics degraded)
// rather than the construction failing.
func TestNewServerDegradesWithoutMetrics(t *testing.T) {
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	telset := telemetry.NoopSettings()
	telset.MeterProvider = &failingMeterProvider{failCounter: true}

	srv, err := NewServer(telset, svc, DefaultConfig())
	require.NoError(t, err)
	require.NotNil(t, srv)
}

// TestRegisterTools verifies RegisterTools advertises the full tool set on a
// bare server (in-memory transport, no HTTP stack). Registration only, so the
// QueryService is backed by empty mocks that are never invoked.
func TestRegisterTools(t *testing.T) {
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil)
	_, err := registerTools(server, svc, DefaultConfig(), zap.NewNop())
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer clientSession.Close()

	listed, err := clientSession.ListTools(ctx, &mcp.ListToolsParams{})
	require.NoError(t, err)

	got := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		got = append(got, tool.Name)
	}

	assert.ElementsMatch(t, []string{
		"get_services", "get_span_names", "search_traces", "get_span_details",
		"get_trace_errors", "get_trace_topology", "get_critical_path", "get_service_dependencies",
		"read_skill",
	}, got)
}

// TestNewHandler_CallTool_OperatorSkill drives a full HTTP+MCP round trip
// through read_skill against an operator-supplied SkillsDir, confirming the
// custom/ skill is reachable end-to-end (not just via buildMergedSkillsFS in
// isolation, as skills_fs_test.go already covers).
func TestNewHandler_CallTool_OperatorSkill(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "slow-db-call/SKILL.md", validSkillMD("slow-db-call"))

	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	cfg := DefaultConfig()
	cfg.SkillsDir = dir
	handler, err := NewHandler(telemetry.NoopSettings(), svc, tenancy.NewManager(&tenancy.Options{}), cfg)
	require.NoError(t, err)

	session := connectTestClient(t, handler)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "read_skill",
		Arguments: map[string]any{"path": "custom/slow-db-call/SKILL.md"},
	})
	require.NoError(t, err)
	require.False(t, result.IsError)

	require.NotEmpty(t, result.Content)
	text, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, text.Text, "name: slow-db-call")
}

// TestNewHandler_InvalidSkillsDirPath asserts the Option B hard-fail half:
// an unusable skills_dir path aborts construction of the handler.
func TestNewHandler_InvalidSkillsDirPath(t *testing.T) {
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	cfg := DefaultConfig()
	cfg.SkillsDir = filepath.Join(t.TempDir(), "no-such-dir")

	_, err := NewHandler(telemetry.NoopSettings(), svc, tenancy.NewManager(&tenancy.Options{}), cfg)
	require.ErrorContains(t, err, "cannot open skills_dir")
}

// TestNewHandler_ExcludesInvalidOperatorSkill is the core Option B test: one
// malformed skill alongside one good skill must not fail Jaeger startup —
// only the bad skill is excluded, and read_skill on it returns an MCP-level
// error rather than surfacing anywhere else.
func TestNewHandler_ExcludesInvalidOperatorSkill(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "good-skill/SKILL.md", validSkillMD("good-skill"))
	writeSkillFile(t, dir, "bad-skill/SKILL.md", "---\nname: MISMATCH\n---\nbody\n")

	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	cfg := DefaultConfig()
	cfg.SkillsDir = dir

	core, logs := observer.New(zap.WarnLevel)
	telset := telemetry.NoopSettings()
	telset.Logger = zap.New(core)

	handler, err := NewHandler(telset, svc, tenancy.NewManager(&tenancy.Options{}), cfg)
	require.NoError(t, err, "one bad operator skill must not fail construction")

	warnings := logs.FilterMessage("skipping invalid operator skill").All()
	require.Len(t, warnings, 1)
	assert.Equal(t, "bad-skill/SKILL.md", warnings[0].ContextMap()["file"])

	session := connectTestClient(t, handler)

	goodResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "read_skill",
		Arguments: map[string]any{"path": "custom/good-skill/SKILL.md"},
	})
	require.NoError(t, err)
	assert.False(t, goodResult.IsError, "the good skill must still serve")

	badResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "read_skill",
		Arguments: map[string]any{"path": "custom/bad-skill/SKILL.md"},
	})
	require.NoError(t, err)
	assert.True(t, badResult.IsError, "the excluded skill must surface as a tool-level error")
}
