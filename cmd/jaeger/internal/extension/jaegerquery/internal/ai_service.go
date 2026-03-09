// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/ollama"
)

// TraceReader allows the AIService to fetch raw traces for contextual LLM summaries.
type TraceReader interface {
	GetTrace(ctx context.Context, traceID string) (*model.Trace, error)
}

// LLMClient defines the interface for communicating with a local LLM.
// The default implementation uses Ollama (port 11434) via LangChainGo.
// StubLLMClient is provided for testing only.
type LLMClient interface {
	// ExtractParameters sends a prompt and forces JSON-formatted extraction.
	ExtractParameters(ctx context.Context, prompt string) (string, error)
	// SummarizeTrace sends a prompt for contextual analysis and returns Markdown text.
	SummarizeTrace(ctx context.Context, prompt string) (string, error)
}

// AIService orchestrates the Phase 2/3 AI trace analysis pipeline:
// 1. Natural language search mapped to JSON parameters.
// 2. Contextual trace summarization from raw database trace fetching.
type AIService struct {
	traceReader TraceReader
	llmClient   LLMClient
}

// NewAIService creates a new AIService with the given TraceReader and LLM clients.
// If llmClient is nil, it gracefully defaults to an Ollama LLM client for local processing.
func NewAIService(traceReader TraceReader, llmClient LLMClient) *AIService {
	if llmClient == nil {
		model := "phi3" // default small language model
		llm, err := ollama.New(ollama.WithModel(model))
		if err == nil {
			llmClient = &OllamaLLMClient{llm: llm, model: model}
		} else {
			log.Printf("WARNING: failed to connect to Ollama (%v); falling back to stub LLM client — AI responses will be fabricated", err)
			llmClient = &StubLLMClient{}
		}
	}

	return &AIService{
		traceReader: traceReader,
		llmClient:   llmClient,
	}
}

// ExtractedSearchParams represents the SLM's structured translation of a natural language search query.
type ExtractedSearchParams struct {
	Service     string            `json:"service"`
	Operation   string            `json:"operation"`
	Tags        map[string]string `json:"tags"`
	MinDuration string            `json:"minDuration"`
	MaxDuration string            `json:"maxDuration"`
}

// GenerateSearchParams takes a natural language query and uses the SLM to extract Jaeger search parameters safely.
func (s *AIService) GenerateSearchParams(ctx context.Context, question string) (*ExtractedSearchParams, error) {
	prompt := buildSearchAnalysisPrompt(question)
	answer, err := s.llmClient.ExtractParameters(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("SLM extraction failed: %w", err)
	}

	var params ExtractedSearchParams
	// Act as a firewall: safely unmarshal only known fields, discarding hallucinations
	if err := json.Unmarshal([]byte(answer), &params); err != nil {
		return nil, fmt.Errorf("SLM returned invalid or unparseable JSON: %w", err)
	}

	return &params, nil
}

// AnalyzeTrace reads the full trace, prunes it to fit LLM constraints, builds a prompt, and queries the LLM.
func (s *AIService) AnalyzeTrace(ctx context.Context, traceID string, question string) (string, error) {
	if s.traceReader == nil {
		return "", fmt.Errorf("TraceReader is not configured for the AI Service")
	}

	trace, err := s.traceReader.GetTrace(ctx, traceID)
	if err != nil {
		return "", fmt.Errorf("failed to fetch trace %s from storage: %w", traceID, err)
	}

	prunedTrace := PruneTraceForLLM(trace)
	prompt := buildContextualPrompt(prunedTrace, question)

	answer, err := s.llmClient.SummarizeTrace(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("LLM trace summarization failed: %w", err)
	}

	return answer, nil
}

// --- Stub implementations (for PoC / testing) ---

// StubTraceReader returns a fake trace for unit testing.
type StubTraceReader struct{}

// GetTrace returns a stub trace.
func (*StubTraceReader) GetTrace(_ context.Context, traceID string) (*model.Trace, error) {
	// Generate a minimal mock trace with an error tag
	trace := &model.Trace{
		Spans: []*model.Span{
			{
				TraceID:       model.NewTraceID(0, 123),
				SpanID:        model.NewSpanID(456),
				OperationName: "api-gateway",
				Tags: []model.KeyValue{
					{Key: "error", VType: model.ValueType_BOOL, VBool: true},
					{Key: "http.status_code", VType: model.ValueType_INT64, VInt64: 500},
				},
			},
		},
	}
	return trace, nil
}

// OllamaLLMClient connects to a local Ollama instance using langchaingo.
type OllamaLLMClient struct {
	llm   *ollama.LLM
	model string
}

// ExtractParameters sends the structured prompt to the local SLM via langchaingo forcing JSON mode.
func (c *OllamaLLMClient) ExtractParameters(ctx context.Context, prompt string) (string, error) {
	completion, err := llms.GenerateFromSinglePrompt(ctx, c.llm, prompt, llms.WithJSONMode())
	if err != nil {
		return "", fmt.Errorf("Ollama SLM JSON generation failed (ensure 'ollama run %s' is active): %w", c.model, err)
	}
	return completion, nil
}

// SummarizeTrace sends the pruned trace text prompt to the SLM natively (Markdown format).
func (c *OllamaLLMClient) SummarizeTrace(ctx context.Context, prompt string) (string, error) {
	completion, err := llms.GenerateFromSinglePrompt(ctx, c.llm, prompt)
	if err != nil {
		return "", fmt.Errorf("Ollama SLM trace summarization failed: %w", err)
	}
	return completion, nil
}

// StubLLMClient returns fixed analysis without calling a real LLM.
type StubLLMClient struct{}

// ExtractParameters returns a stub JSON extraction.
func (*StubLLMClient) ExtractParameters(_ context.Context, _ string) (string, error) {
	return `{"service": "payment-service", "operation": "checkout", "tags": {"error": "true"}}`, nil
}

// SummarizeTrace returns a stub analysis string.
func (*StubLLMClient) SummarizeTrace(_ context.Context, _ string) (string, error) {
	return "The trace shows the payment-service is the primary bottleneck. Consider investigating the payment-service for slow database queries.", nil
}
