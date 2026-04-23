// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"encoding/json"
	"sync"
)

// ContextualToolsStore stores the AG-UI tools that the frontend provided for
// a given ACP session. The chat handler populates it on each request and
// the gateway-hosted MCP endpoint (see contextual_mcp.go) reads the
// snapshot for the requested session. The session ID is the correlation
// key so concurrent turns from different frontends cannot clobber each
// other's snapshots.
//
// Entries are kept as []json.RawMessage so that GetContextualToolsForSession
// can unmarshal a fresh tree per reader. That guarantees callers cannot
// corrupt the stored snapshot by mutating decoded maps.
type ContextualToolsStore struct {
	mu        sync.RWMutex
	bySession map[string][]json.RawMessage
}

// NewContextualToolsStore creates a ready-to-use store.
func NewContextualToolsStore() *ContextualToolsStore {
	return &ContextualToolsStore{bySession: make(map[string][]json.RawMessage)}
}

// SetForSession stores frontend-provided AG-UI tools keyed by ACP session
// ID. Entries that do not parse as JSON are skipped. The raw bytes are
// copied so that later mutations of the caller's slice cannot affect the
// stored snapshot.
func (s *ContextualToolsStore) SetForSession(sessionID string, rawTools []json.RawMessage) {
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
	s.bySession[sessionID] = valid
	s.mu.Unlock()
}

// DeleteForSession drops the tools snapshot for the given ACP session. The
// chat handler must call this once the turn has finished (success or
// failure) so the store does not accumulate entries across the lifetime of
// the query process.
func (s *ContextualToolsStore) DeleteForSession(sessionID string) {
	if sessionID == "" {
		return
	}
	s.mu.Lock()
	delete(s.bySession, sessionID)
	s.mu.Unlock()
}

// GetContextualToolsForSession returns a freshly-unmarshalled copy of the
// tools snapshot stored for the given ACP session. Each call produces a new
// tree, so callers cannot corrupt the stored snapshot by mutating nested
// maps. Returns nil when the session is unknown or has no registered tools.
func (s *ContextualToolsStore) GetContextualToolsForSession(sessionID string) []any {
	if sessionID == "" {
		return nil
	}

	s.mu.RLock()
	raws := s.bySession[sessionID]
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
