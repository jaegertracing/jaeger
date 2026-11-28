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
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/querysvc"
	depstoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore/mocks"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/telemetry/otelsemconv"
)

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

// toolNamePattern is the naming convention of ADR-002: a verb and its direct object,
// where the verb is get for a lookup, find for a search, or read for a document.
var toolNamePattern = regexp.MustCompile(`^(get|find|read)_[a-z]+(_[a-z]+)*$`)

// TestRegisterTools verifies that every registered tool reaches a client through
// tools/list on a bare server (in-memory transport, no HTTP stack) with a name that
// follows the convention, a description, and the read-only, closed-world annotations.
// Registration only, so the QueryService is backed by empty mocks that are never invoked.
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
	require.NotEmpty(t, listed.Tools)

	names := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
		assert.Regexp(t, toolNamePattern, tool.Name)
		assert.NotEmpty(t, tool.Description, tool.Name)
		assert.NotNil(t, tool.InputSchema, tool.Name)
		require.NotNil(t, tool.Annotations, tool.Name)
		assert.True(t, tool.Annotations.ReadOnlyHint, tool.Name)
		require.NotNil(t, tool.Annotations.OpenWorldHint, tool.Name)
		assert.False(t, *tool.Annotations.OpenWorldHint, tool.Name)
	}
	assert.ElementsMatch(t, registeredToolNames, names)
}

// TestMetricsUnknownToolName calls tools the server does not have, through the
// real middleware chain, and checks that their names do not become label values.
func TestMetricsUnknownToolName(t *testing.T) {
	capture := newMetricsCapture(t)
	telset := telemetry.NoopSettings()
	telset.MeterProvider = capture.provider
	svc := querysvc.NewQueryService(&tracestoremocks.Reader{}, &depstoremocks.Reader{}, querysvc.QueryServiceOptions{})
	server := newServer(telset, svc, DefaultServerParams())

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

	for _, name := range []string{"nope_1", "nope_2"} {
		_, callErr := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: name})
		require.ErrorContains(t, callErr, "unknown tool")
	}

	toolKey := otelsemconv.GenAIToolName("").Key
	var toolNames []string
	calls := findMetricDataPoint[metricdata.Sum[int64]](t, capture.collect(t), "jaeger.mcp.tool.calls")
	for _, dp := range calls.DataPoints {
		if v, ok := dp.Attributes.Value(toolKey); ok {
			toolNames = append(toolNames, v.AsString())
			assert.Equal(t, int64(2), dp.Value)
		}
	}
	assert.Equal(t, []string{metricToolNameOther}, toolNames)
}

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
