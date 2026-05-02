// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"encoding/json"
	"sync"
)

// ContextualToolsStore stores the AG-UI tools that the frontend provided
// for a given ACP turn. The chat handler mints a per-turn contextual MCP
// id and uses it as the key into this store; the dispatcher (see
// dispatcher.go) reads the snapshot for that id when the sidecar
// dispatches a contextual tool call back via the
// ExtMethodJaegerToolCall ACP extension method.
//
// The contextual MCP id is *not* the ACP session id: the latter is
// assigned by the sidecar after NewSession returns, which is too late to
// embed in NewSessionRequest.Meta. The contextual MCP id is minted
// gateway-side before the request leaves and is the correlation key for
// concurrent turns.
//
// Entries are kept as []json.RawMessage so that GetContextualToolsForID
// can unmarshal a fresh tree per reader. That guarantees callers cannot
// corrupt the stored snapshot by mutating decoded maps.
type ContextualToolsStore struct {
	mu   sync.RWMutex
	byID map[string][]json.RawMessage
}

// NewContextualToolsStore creates a ready-to-use store.
func NewContextualToolsStore() *ContextualToolsStore {
	return &ContextualToolsStore{byID: make(map[string][]json.RawMessage)}
}

// SetForContextualMCPID stores frontend-provided AG-UI tools keyed by
// the per-turn contextual MCP id. Entries that do not parse as JSON are
// skipped. The raw bytes are copied so that later mutations of the
// caller's slice cannot affect the stored snapshot. An empty id is
// treated as a no-op so callers cannot accidentally write an entry
// under "" that Get/Delete refuse to touch.
//
// If every entry is invalid (or rawTools is empty/nil) the call deletes
// any existing entry for the id rather than writing an empty slice;
// otherwise the map would accumulate keys that GetContextualToolsForID
// reports as "no snapshot" but that DeleteForContextualMCPID is the
// only way to ever remove.
func (s *ContextualToolsStore) SetForContextualMCPID(contextualMCPID string, rawTools []json.RawMessage) {
	if contextualMCPID == "" {
		return
	}
	valid := make([]json.RawMessage, 0, len(rawTools))
	for _, raw := range rawTools {
		var probe any
		if err := json.Unmarshal(raw, &probe); err != nil {
			continue
		}
		cloned := make(json.RawMessage, len(raw))
		copy(cloned, raw)
		valid = append(valid, cloned)
	}

	s.mu.Lock()
	if len(valid) == 0 {
		delete(s.byID, contextualMCPID)
	} else {
		s.byID[contextualMCPID] = valid
	}
	s.mu.Unlock()
}

// DeleteForContextualMCPID drops the tools snapshot for the given turn.
// The chat handler must call this once the turn has finished (success or
// failure) so the store does not accumulate entries across the lifetime of
// the query process.
func (s *ContextualToolsStore) DeleteForContextualMCPID(contextualMCPID string) {
	if contextualMCPID == "" {
		return
	}
	s.mu.Lock()
	delete(s.byID, contextualMCPID)
	s.mu.Unlock()
}

// GetContextualToolsForID returns a freshly-unmarshalled copy of the tools
// snapshot stored under the given contextual MCP id. Each call produces a
// new tree, so callers cannot corrupt the stored snapshot by mutating
// nested maps. Returns nil when the id is unknown or has no registered
// tools.
func (s *ContextualToolsStore) GetContextualToolsForID(contextualMCPID string) []any {
	if contextualMCPID == "" {
		return nil
	}

	s.mu.RLock()
	raws := s.byID[contextualMCPID]
	s.mu.RUnlock()

	if len(raws) == 0 {
		return nil
	}

	result := make([]any, 0, len(raws))
	for _, raw := range raws {
		var tool any
		if err := json.Unmarshal(raw, &tool); err != nil {
			continue
		}
		result = append(result, tool)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
