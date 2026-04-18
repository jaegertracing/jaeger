// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"encoding/json"
	"sync"
)

// ContextualToolsStore stores the AG-UI tools that the frontend provided for
// a given ACP session. The chat handler populates it on each request and
// the MCP list_contextual_tools tool reads the snapshot for the requested
// session. The session ID is the correlation key so concurrent turns from
// different frontends cannot clobber each other's snapshots.
type ContextualToolsStore struct {
	mu        sync.RWMutex
	bySession map[string][]any
}

// NewContextualToolsStore creates a ready-to-use store.
func NewContextualToolsStore() *ContextualToolsStore {
	return &ContextualToolsStore{bySession: make(map[string][]any)}
}

// SetForSession stores frontend-provided AG-UI tools keyed by ACP session
// ID. Tools are decoded from raw JSON so the caller is not constrained to a
// specific schema.
func (s *ContextualToolsStore) SetForSession(sessionID string, rawTools []json.RawMessage) {
	decoded := make([]any, 0, len(rawTools))
	for _, raw := range rawTools {
		var tool any
		if err := json.Unmarshal(raw, &tool); err != nil {
			continue
		}
		decoded = append(decoded, tool)
	}

	s.mu.Lock()
	s.bySession[sessionID] = decoded
	s.mu.Unlock()
}

// GetContextualToolsForSession returns a copy of the tools snapshot stored
// for the given ACP session. It returns nil when the session is unknown or
// has no registered tools.
func (s *ContextualToolsStore) GetContextualToolsForSession(sessionID string) []any {
	if sessionID == "" {
		return nil
	}

	s.mu.RLock()
	tools := s.bySession[sessionID]
	s.mu.RUnlock()

	if len(tools) == 0 {
		return nil
	}

	result := make([]any, len(tools))
	copy(result, tools)
	return result
}
