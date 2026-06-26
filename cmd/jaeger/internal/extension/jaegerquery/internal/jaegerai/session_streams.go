// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import "sync"

// SessionStreams holds the per-session SSE streaming client so the MCP
// proxy can find it and dispatch UI-tool calls to the right browser.
//
// The chat handler registers a `*streamingClient` here under the ACP
// session id as soon as the session is created, and removes it at end of
// turn. The MCP proxy looks the streaming client up by the session id it
// parses from `/api/ai/mcp/<sessionId>/*` to fire TOOL_CALL_* events when
// the agent invokes a contextual tool.
//
// The registry is in-memory and tied to the lifetime of the Jaeger
// process. We don't persist it because:
//   - ACP sessions are scoped to a single HTTP chat request — they don't
//     survive process restarts.
//   - The streaming client is a live `http.ResponseWriter` wrapper;
//     persisting it across processes is nonsensical.
//
// Methods are safe for concurrent use. The chat handler writes from the
// HTTP request goroutine; the MCP proxy reads from arbitrary HTTP
// goroutines spawned per MCP request. A plain RWMutex is fine — the map
// is small (one entry per active chat) and contended for microseconds.
type SessionStreams struct {
	mu      sync.RWMutex
	streams map[string]*streamingClient
}

// NewSessionStreams returns an empty registry.
func NewSessionStreams() *SessionStreams {
	return &SessionStreams{
		streams: make(map[string]*streamingClient),
	}
}

// Set records the streaming client for the given session id. Overwrites
// any prior entry — the gateway never reuses session ids across chat
// turns, so a collision indicates a programming error in the caller, not
// a normal interleaving we need to handle.
func (s *SessionStreams) Set(sessionID string, client *streamingClient) {
	if sessionID == "" || client == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.streams[sessionID] = client
}

// Get returns the streaming client for sessionID, or nil if no chat is
// active for that id. Returning nil instead of an error keeps the MCP
// proxy's call site cheap: a stale or unknown session id is a "tool can't
// be dispatched" case, not a structural error.
//
//nolint:revive // streamingClient is intentionally unexported — SessionStreams is only consumed inside this package.
func (s *SessionStreams) Get(sessionID string) *streamingClient {
	if sessionID == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.streams[sessionID]
}

// Delete removes the entry for sessionID. Idempotent: called by the chat
// handler's defer block, so it must not panic if Set never ran (e.g. when
// session/new failed).
func (s *SessionStreams) Delete(sessionID string) {
	if sessionID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.streams, sessionID)
}
