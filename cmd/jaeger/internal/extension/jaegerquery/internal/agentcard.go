// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"encoding/json"
	"net/http"
)

// AgentCard is the JSON payload served at /.well-known/agent.json.
// It follows the A2A agent discovery specification, advertising this Jaeger
// instance as an AI-capable agent that external agentic tools can connect to.
type AgentCard struct {
	// Name is the human-readable name of this agent.
	Name string `json:"name"`
	// Description describes what this agent specialises in.
	Description string `json:"description"`
	// Endpoint is the A2A-compatible HTTP endpoint for submitting tasks.
	// Empty until the A2A gateway is implemented.
	Endpoint string `json:"endpoint,omitempty"`
	// Skills lists the high-level analytical capabilities offered.
	Skills []string `json:"skills"`
	// MCPServers lists the MCP server URLs that back this agent's tools.
	MCPServers []string `json:"mcpServers"`
}

// agentCardHandler serves GET /.well-known/agent.json with a static AgentCard.
type agentCardHandler struct {
	card AgentCard
}

func newAgentCardHandler(mcpEndpoint string) *agentCardHandler {
	servers := []string{}
	if mcpEndpoint != "" {
		servers = append(servers, mcpEndpoint)
	}
	return &agentCardHandler{
		card: AgentCard{
			Name:        "Jaeger Diagnostic Copilot",
			Description: "Expert in distributed tracing and root cause analysis using Jaeger.",
			Skills:      []string{"trace_analysis", "bottleneck_detection", "error_diagnosis"},
			MCPServers:  servers,
		},
	}
}

func (h *agentCardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(h.card); err != nil {
		http.Error(w, "failed to encode agent card", http.StatusInternalServerError)
	}
}
