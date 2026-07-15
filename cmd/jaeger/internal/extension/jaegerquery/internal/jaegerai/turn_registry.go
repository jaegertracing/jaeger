// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"encoding/json"
	"sync"
)

// turnState holds the per-turn state the turn-scoped MCP endpoint needs: the live
// SSE stream back to the browser, and the UI tools the frontend declared for that
// turn. The endpoint advertises those UI tools (alongside the built-in telemetry
// tools) and dispatches their calls onto the stream.
type turnState struct {
	stream  *streamingClient
	uiTools []json.RawMessage
}

// turnRegistry maps a per-turn route id (mcpRouteID) to its turn state. The chat
// handler mints a route id when it opens a turn, registers the stream and the
// turn's UI tools here, and removes the entry when the turn ends. The turn-scoped
// MCP endpoint (`/api/ai/mcp/<mcpRouteID>/`) looks the id up to confirm it belongs
// to an active turn and to build that turn's tool set.
//
// It is the join key between the two halves of a turn: the browser's SSE stream
// (held by the chat handler) and the sidecar's MCP connection (dialed at the
// turn-scoped URL).
//
// The registry is in-memory and tied to the process lifetime — turns are scoped
// to a single HTTP chat request and the streaming client wraps a live
// http.ResponseWriter, so there is nothing meaningful to persist.
//
// Methods are safe for concurrent use: the chat handler writes from the request
// goroutine while the MCP endpoint reads from per-request goroutines. The map is
// small (one entry per active chat) so a plain RWMutex is ample.
type turnRegistry struct {
	mu    sync.RWMutex
	turns map[string]*turnState
}

// newTurnRegistry returns an empty registry.
func newTurnRegistry() *turnRegistry {
	return &turnRegistry{
		turns: make(map[string]*turnState),
	}
}

// set records the stream and UI tools for mcpRouteID. No-op on empty id or nil
// stream. Route ids are never reused across turns, so an overwrite indicates
// a caller bug rather than a normal interleaving.
func (s *turnRegistry) set(mcpRouteID string, stream *streamingClient, uiTools []json.RawMessage) {
	if mcpRouteID == "" || stream == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.turns[mcpRouteID] = &turnState{stream: stream, uiTools: uiTools}
}

// get returns the turn state for mcpRouteID, or nil when no turn is active for it.
// Returning nil (rather than an error) keeps the endpoint's call site cheap: an
// unknown or expired id is a "not an active turn" case, which the endpoint
// maps to 404.
func (s *turnRegistry) get(mcpRouteID string) *turnState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.turns[mcpRouteID] // nil for "" or an unknown id
}

// delete removes mcpRouteID. Idempotent: it runs from the chat handler's defer,
// so it must not panic if set never ran (e.g. session/new failed).
func (s *turnRegistry) delete(mcpRouteID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.turns, mcpRouteID) // no-op for "" or an unknown id
}

// count returns the number of active turns. Used by tests to observe the
// register/deregister lifecycle without reaching into the guarded map.
func (s *turnRegistry) count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.turns)
}
