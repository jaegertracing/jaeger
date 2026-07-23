// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"encoding/json"
	"sync"

	"github.com/google/uuid"
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

// register records a new turn — the browser's SSE stream and the UI tools it
// declared — under a freshly minted route id, and returns that id plus a closer
// that removes the turn. The registry mints the id (nothing outside chooses how a
// turn is keyed); the closer is meant to be deferred by the chat handler and is
// idempotent. The chat handler ignores the id today — the turn-scoped endpoint
// resolves it from the request path — but the id is what the turn's MCP URL is
// built from once that URL is announced to the sidecar.
func (s *turnRegistry) register(stream *streamingClient, uiTools []json.RawMessage) (mcpRouteID string, closer func()) {
	id := uuid.NewString()
	s.mu.Lock()
	s.turns[id] = &turnState{stream: stream, uiTools: uiTools}
	s.mu.Unlock()
	return id, func() {
		s.mu.Lock()
		delete(s.turns, id)
		s.mu.Unlock()
	}
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

// count returns the number of active turns. Used by tests to observe the
// register/deregister lifecycle without reaching into the guarded map.
func (s *turnRegistry) count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.turns)
}
