// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// analyzeRequest is the JSON body for POST /api/ai/analyze.
type analyzeRequest struct {
	TraceID  string `json:"trace_id"`
	Question string `json:"question"`
}

// analyzeResponse is returned inside structuredResponse.Data.
type analyzeResponse struct {
	TraceID string `json:"trace_id"`
	Answer  string `json:"answer"`
}

// analyzeTraceAI handles POST /api/ai/analyze requests.
// It accepts a trace_id and question, orchestrates MCP + LLM calls via AIService,
// and returns the analysis wrapped in the standard structuredResponse envelope.
//
// This is a Level-1 AI endpoint (single-trace Q&A) as described in:
// - Issue #7832 (AI-Powered Trace Analysis with Local LLM Support)
// - Issue #7827 (GenAI integration with Jaeger, Level 1)
// - ADR-002 (MCP Server)
func (aH *APIHandler) analyzeTraceAI(w http.ResponseWriter, r *http.Request) {
	var req analyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		aH.handleError(w, fmt.Errorf("invalid JSON request body: %w", err), http.StatusBadRequest)
		return
	}

	if req.TraceID == "" {
		aH.handleError(w, errors.New("trace_id is required"), http.StatusBadRequest)
		return
	}

	if req.Question == "" {
		aH.handleError(w, errors.New("question is required"), http.StatusBadRequest)
		return
	}

	if aH.aiService == nil {
		aH.handleError(w, errors.New("AI service is not configured"), http.StatusNotImplemented)
		return
	}

	answer, err := aH.aiService.AnalyzeTrace(r.Context(), req.TraceID, req.Question)
	if aH.handleError(w, err, http.StatusInternalServerError) {
		return
	}

	resp := structuredResponse{
		Data: analyzeResponse{
			TraceID: req.TraceID,
			Answer:  answer,
		},
		Total: 1,
	}

	aH.writeJSON(w, r, &resp)
}
