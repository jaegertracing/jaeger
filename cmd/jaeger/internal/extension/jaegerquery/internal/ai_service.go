// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
)

// MCPClient defines the interface for communicating with the Jaeger MCP server.
// In production, this will make HTTP calls to the MCP server (default port 16687).
// TODO: Replace StubMCPClient with an HTTP client calling MCP tools on ports.MCPHTTP.
type MCPClient interface {
	// GetTraceTopology returns the service dependency topology for a given trace.
	GetTraceTopology(ctx context.Context, traceID string) (string, error)
	// GetCriticalPath returns the critical path analysis for a given trace.
	GetCriticalPath(ctx context.Context, traceID string) (string, error)
}

// LLMClient defines the interface for communicating with a local LLM.
// In production, this will call Ollama (default port 11434) or similar local models.
// TODO: Replace StubLLMClient with an Ollama/LangChainGo client.
type LLMClient interface {
	// AnalyzeTrace sends a prompt containing trace context to the LLM and returns its analysis.
	AnalyzeTrace(ctx context.Context, prompt string) (string, error)
}

// AIService orchestrates the Level-1 AI trace analysis pipeline:
// 1. Fetch trace context from MCP tools (topology, critical path).
// 2. Build a prompt combining trace context with the user's question.
// 3. Send the prompt to the LLM for analysis.
type AIService struct {
	mcpClient MCPClient
	llmClient LLMClient
}

// NewAIService creates a new AIService with the given MCP and LLM clients.
func NewAIService(mcpClient MCPClient, llmClient LLMClient) *AIService {
	return &AIService{
		mcpClient: mcpClient,
		llmClient: llmClient,
	}
}

// AnalyzeTrace performs Level-1 single-trace Q&A:
// fetches trace topology and critical path via MCP, builds a prompt, and queries the LLM.
func (s *AIService) AnalyzeTrace(ctx context.Context, traceID string, question string) (string, error) {
	topology, err := s.mcpClient.GetTraceTopology(ctx, traceID)
	if err != nil {
		return "", fmt.Errorf("failed to get trace topology: %w", err)
	}

	criticalPath, err := s.mcpClient.GetCriticalPath(ctx, traceID)
	if err != nil {
		return "", fmt.Errorf("failed to get critical path: %w", err)
	}

	prompt := buildAnalysisPrompt(traceID, topology, criticalPath, question)

	answer, err := s.llmClient.AnalyzeTrace(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("LLM analysis failed: %w", err)
	}

	return answer, nil
}

// buildAnalysisPrompt constructs the LLM prompt from trace context and user question.
func buildAnalysisPrompt(traceID, topology, criticalPath, question string) string {
	return fmt.Sprintf(
		"You are an expert distributed systems engineer analyzing a trace.\n\n"+
			"Trace ID: %s\n\n"+
			"Trace Topology:\n%s\n\n"+
			"Critical Path:\n%s\n\n"+
			"User Question: %s\n\n"+
			"Provide a concise, actionable analysis.",
		traceID, topology, criticalPath, question,
	)
}

// --- Stub implementations (for PoC / testing) ---

// StubMCPClient returns fixed responses without calling a real MCP server.
type StubMCPClient struct{}

// GetTraceTopology returns a stub topology string.
func (*StubMCPClient) GetTraceTopology(_ context.Context, traceID string) (string, error) {
	return fmt.Sprintf(
		"frontend-web -> api-gateway -> order-service -> payment-service (trace: %s)",
		traceID,
	), nil
}

// GetCriticalPath returns a stub critical path string.
func (*StubMCPClient) GetCriticalPath(_ context.Context, traceID string) (string, error) {
	return fmt.Sprintf(
		"api-gateway (12ms) -> order-service (45ms) -> payment-service (120ms) [total: 177ms] (trace: %s)",
		traceID,
	), nil
}

// StubLLMClient returns a fixed analysis without calling a real LLM.
type StubLLMClient struct{}

// AnalyzeTrace returns a stub analysis string.
func (*StubLLMClient) AnalyzeTrace(_ context.Context, _ string) (string, error) {
	return "The trace shows the payment-service is the primary bottleneck, " +
		"consuming 120ms (68% of the total 177ms critical path). " +
		"Consider investigating the payment-service for slow database queries or external API calls.", nil
}
