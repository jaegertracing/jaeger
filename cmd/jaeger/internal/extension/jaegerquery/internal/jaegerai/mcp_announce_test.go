// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func httpCaps(supported bool) acp.AgentCapabilities {
	return acp.AgentCapabilities{
		McpCapabilities: acp.McpCapabilities{Http: supported},
	}
}

const testBaseURL = "https://jaeger.example.com:16686"

func TestAnnounceMCPServersOverHTTP(t *testing.T) {
	got := announceMCPServers(httpCaps(true), testBaseURL, "", "route-1")
	require.Len(t, got, 1)
	require.NotNil(t, got[0].Http)
	assert.Equal(t, "http", got[0].Http.Type)
	assert.Equal(t, mcpServerName, got[0].Http.Name)
	assert.Equal(t, testBaseURL+"/api/ai/mcp/route-1/", got[0].Http.Url)
}

// TestAnnounceMCPServersRequiresHTTPCapability is the core contract: ACP requires
// an agent to opt into each McpServer variant, so announcing HTTP to an agent that
// cannot consume it would make it fail the session.
func TestAnnounceMCPServersRequiresHTTPCapability(t *testing.T) {
	got := announceMCPServers(httpCaps(false), testBaseURL, "", "route-1")
	assert.Empty(t, got, "an agent that did not advertise mcpCapabilities.http is offered nothing")
}

// TestAnnounceMCPServersRequiresBaseURL is why ai.mcp_base_url exists: the gateway
// cannot infer an address the sidecar can reach it on, and announcing a wrong one
// would make the agent dial it and fail mid-turn. Announcing nothing is safer.
func TestAnnounceMCPServersRequiresBaseURL(t *testing.T) {
	got := announceMCPServers(httpCaps(true), "", "", "route-1")
	assert.Empty(t, got, "with no configured base URL the endpoint is not announced")
}

func TestAnnounceMCPServersRequiresRouteID(t *testing.T) {
	got := announceMCPServers(httpCaps(true), testBaseURL, "", "")
	assert.Empty(t, got, "without a route id there is no turn-scoped endpoint to announce")
}

func TestAnnounceMCPServersEmbedsBasePath(t *testing.T) {
	got := announceMCPServers(httpCaps(true), "http://127.0.0.1:16686", "/jaeger", "u-1")
	require.Len(t, got, 1)
	assert.Equal(t, "http://127.0.0.1:16686/jaeger/api/ai/mcp/u-1/", got[0].Http.Url,
		"the announced URL must carry the query server's base path")
}
