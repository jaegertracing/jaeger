// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package mcptools

import (
	"context"
	"encoding/json"
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

	for _, tool := range listed.Tools {
		assert.Regexp(t, toolNamePattern, tool.Name)
		assert.NotEmpty(t, tool.Description, tool.Name)
		assert.NotNil(t, tool.InputSchema, tool.Name)
		require.NotNil(t, tool.Annotations, tool.Name)
		assert.True(t, tool.Annotations.ReadOnlyHint, tool.Name)
		require.NotNil(t, tool.Annotations.OpenWorldHint, tool.Name)
		assert.False(t, *tool.Annotations.OpenWorldHint, tool.Name)
	}

	byName := toolsByName(t, listed.Tools)
	errorSpanProps := spanItemProperties(t, byName["get_trace_errors"])
	detailSpanProps := spanItemProperties(t, byName["get_span_details"])

	for _, field := range []string{"attributes", "events", "links"} {
		assert.NotContains(t, errorSpanProps, field,
			"get_trace_errors output schema must not advertise %s on returned spans", field)
		assert.Contains(t, detailSpanProps, field,
			"get_span_details output schema must still advertise %s", field)
	}
	for _, field := range []string{"span_id", "service", "span_name", "status_message"} {
		assert.Contains(t, errorSpanProps, field,
			"get_trace_errors span listing schema must include %s", field)
	}
	errorOutProps := schemaObject(t, marshalSchema(t, byName["get_trace_errors"].OutputSchema), "properties")
	assert.Contains(t, errorOutProps, "trace_id")
	assert.Contains(t, errorOutProps, "total_error_count")
}

func toolsByName(t *testing.T, tools []*mcp.Tool) map[string]*mcp.Tool {
	t.Helper()
	byName := make(map[string]*mcp.Tool, len(tools))
	for i := range tools {
		byName[tools[i].Name] = tools[i]
	}
	require.Contains(t, byName, "get_trace_errors")
	require.Contains(t, byName, "get_span_details")
	return byName
}

func marshalSchema(t *testing.T, schema any) map[string]any {
	t.Helper()
	require.NotNil(t, schema)
	raw, err := json.Marshal(schema)
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func schemaObject(t *testing.T, node map[string]any, keys ...string) map[string]any {
	t.Helper()
	cur := any(node)
	for _, key := range keys {
		m, ok := cur.(map[string]any)
		require.Truef(t, ok, "schema node for %q is %T, want object", key, cur)
		next, ok := m[key]
		require.Truef(t, ok, "missing schema key %q in %v", key, keys)
		cur = next
	}
	m, ok := cur.(map[string]any)
	require.Truef(t, ok, "final schema node is %T, want object", cur)
	return m
}

func spanItemProperties(t *testing.T, tool *mcp.Tool) map[string]any {
	t.Helper()
	require.NotNil(t, tool)
	return schemaObject(t, marshalSchema(t, tool.OutputSchema), "properties", "spans", "items", "properties")
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
