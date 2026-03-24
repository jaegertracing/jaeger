// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentCardHandler_Get(t *testing.T) {
	h := newAgentCardHandler("http://localhost:4320")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var card AgentCard
	require.NoError(t, json.NewDecoder(w.Body).Decode(&card))
	assert.Equal(t, "Jaeger Diagnostic Copilot", card.Name)
	assert.NotEmpty(t, card.Description)
	assert.NotEmpty(t, card.Skills)
	assert.Equal(t, []string{"http://localhost:4320"}, card.MCPServers)
}

func TestAgentCardHandler_NoMCPEndpoint(t *testing.T) {
	h := newAgentCardHandler("")

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var card AgentCard
	require.NoError(t, json.NewDecoder(w.Body).Decode(&card))
	assert.Empty(t, card.MCPServers)
}

func TestAgentCardHandler_MethodNotAllowed(t *testing.T) {
	h := newAgentCardHandler("")

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/.well-known/agent.json", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		assert.Equal(t, http.StatusMethodNotAllowed, w.Code, "method %s should be rejected", method)
	}
}
