// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"encoding/json"
	"sync"
)

// ContextualToolsStore stores the AG-UI tools that the frontend provided
// for a given ACP turn, keyed by the ACP session id assigned by the
// sidecar. The chat handler writes the snapshot once NewSessionResponse
// returns and before Prompt is sent; the dispatcher (see dispatcher.go)
// reads the snapshot when the sidecar dispatches a contextual tool call
// back via the ExtMethodJaegerToolCall ACP extension method, which carries
// the same session id on its payload.
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

// SetForSession stores frontend-provided AG-UI tools keyed by the ACP
// session id. Entries that do not parse as JSON are skipped. The raw
// bytes are copied so that later mutations of the caller's slice cannot
// affect the stored snapshot. An empty id is treated as a no-op so
// callers cannot accidentally write an entry under "" that Get/Delete
// refuse to touch.
//
// If every entry is invalid (or rawTools is empty/nil) the call deletes
// any existing entry for the id rather than writing an empty slice;
// otherwise the map would accumulate keys that GetContextualToolsForSession
// reports as "no snapshot" but that DeleteForSession is the only way to
// ever remove.
func (s *ContextualToolsStore) SetForSession(sessionID string, rawTools []json.RawMessage) {
	if sessionID == "" {
		return
	}
	valid := make([]json.RawMessage, 0, len(rawTools))
	for _, raw := range rawTools {
		if !json.Valid(raw) {
			continue
		}
		cloned := make(json.RawMessage, len(raw))
		copy(cloned, raw)
		valid = append(valid, cloned)
	}

	s.mu.Lock()
	if len(valid) == 0 {
		delete(s.bySession, sessionID)
	} else {
		s.bySession[sessionID] = valid
	}
	s.mu.Unlock()
}

// DeleteForSession drops the tools snapshot for the given session. The
// chat handler must call this once the turn has finished (success or
// failure) so the store does not accumulate entries across the lifetime
// of the query process.
func (s *ContextualToolsStore) DeleteForSession(sessionID string) {
	if sessionID == "" {
		return
	}
	s.mu.Lock()
	delete(s.bySession, sessionID)
	s.mu.Unlock()
}

// GetContextualToolsForSession returns a freshly-unmarshalled copy of the
// tools snapshot stored under the given ACP session id. Each call produces
// a new tree, so callers cannot corrupt the stored snapshot by mutating
// nested maps. Returns nil when the id is unknown or has no registered
// tools.
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
