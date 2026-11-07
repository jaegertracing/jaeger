// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"context"
	"io/fs"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	depstoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

// connectTestClient starts an httptest server around the handler and returns a
// connected MCP client session.
// TestNewHandler_ListTools drives the full in-process serving stack (mcp.Server
// + middleware + tenancy + otelhttp) over HTTP and asserts the telemetry tools
// are advertised. ListTools does not touch storage, so the QueryService is
// backed by empty mocks.
// TestNewHandler_CallTool exercises a tool end-to-end through the HTTP stack,
// confirming the handler reaches the QueryService and returns a result.
// TestNewHandler_ToolPanicFailsOnlyThatCall drives a panicking tool through the
// full HTTP stack. The SDK runs tool handlers on goroutines of its own, so an
// unrecovered panic there would terminate the test binary. With recovery in place
// the call returns an error result that tracing records as a failed call, and the
// session keeps serving.
// TestNewServerDegradesWithoutMetrics covers the branch where the metrics
// middleware fails to build: the server is still returned (metrics degraded)
// rather than the construction failing.
func TestNewServerDegradesWithoutMetrics(t *testing.T) {
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	telset := telemetry.NoopSettings()
	telset.MeterProvider = &failingMeterProvider{failCounter: true}

	srv := newServer(telset, svc, DefaultServerParams())
	require.NotNil(t, srv)
}

// TestHandlerAddReceivingMiddlewarePanicFailsOnlyThatCall covers middleware the AI
// gateway adds after the server is built. The SDK places it outside the recovery
// newServer installs, so a call it answers itself (a UI tool) never reaches that
// recovery. A panic there must still fail only that call rather than terminate the
// test binary, and the session must keep serving the built-in tools.
// TestRegisterTools verifies RegisterTools advertises the full tool set on a
// bare server (in-memory transport, no HTTP stack). Registration only, so the
// QueryService is backed by empty mocks that are never invoked.
func TestRegisterTools(t *testing.T) {
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.0"}, nil)
	registerTools(server, svc, DefaultServerParams())

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
		"get_services", "get_span_names", "find_traces", "get_span_details",
		"get_trace_errors", "get_trace_topology", "get_critical_path", "get_service_dependencies",
		"read_skill",
	}, got)
}

// TestNewHandler_LocalhostProtection pins the SDK's DNS-rebinding guard and the
// config switch that turns it off. httptest listens on 127.0.0.1, so a request
// that carries a non-loopback Host header is the shape the guard rejects: the
// same shape a sidecar that dials 127.0.0.1 and preserves Host produces. The
// SDK also honours MCPGODEBUG=disablelocalhostprotection=1, read once at process
// start, so the default subtest fails on a machine with that variable set.
// TestNewHandler_Sessionless pins the request model of MCP revision 2026-07-28 on
// the endpoint: no request depends on an earlier one, so a client behind a load
// balancer without session affinity is served by whichever replica its request
// reaches. Each NewHandler stands in for one jaeger-query process.
// TestNewHandler_JSONResponse pins the body form of a single-message POST under
// each setting, and that the go-sdk client, which accepts both forms, completes a
// session against the JSON form.
// TestBuiltinSkillsCatalog checks that every skill the root catalog links to is in the
// binary, because an agent passes the link text straight back to read_skill.
func TestBuiltinSkillsCatalog(t *testing.T) {
	skills, err := fs.Sub(skillsEmbedFS, "skills")
	require.NoError(t, err)
	catalog, err := fs.ReadFile(skills, entryPointSkill)
	require.NoError(t, err)

	links := regexp.MustCompile(`\]\(([^)]+/SKILL\.md)\)`).FindAllStringSubmatch(string(catalog), -1)
	require.NotEmpty(t, links)
	var linked []string
	for _, link := range links {
		if strings.HasPrefix(link[1], "custom/") {
			continue
		}
		linked = append(linked, link[1])
		_, err := fs.Stat(skills, link[1])
		require.NoError(t, err, "catalog links %s", link[1])
	}
	assert.Contains(t, linked, "jaeger-sql/SKILL.md")
}
