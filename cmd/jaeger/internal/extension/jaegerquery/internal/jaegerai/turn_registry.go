// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"encoding/json"
	"sync"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// turnState holds the per-turn state the turn-scoped MCP endpoint needs: the live
// SSE stream back to the browser, the UI tools the frontend declared for that
// turn, and the caller the chat request resolved. The endpoint advertises those
// UI tools (alongside the built-in telemetry tools), dispatches their calls onto
// the stream, and runs every tool call as that caller.
//
// A turn also owns the MCP sessions the agent opened under its route. In the
// stateful transport a session keeps the context of the request that created
// it, caller included, for as long as it lives, and the shared MCP mount serves
// the same sessions by id with no route check (only the ai.auth check, when one
// is configured), so the turn closes its sessions when it ends rather than let
// the caller's identity outlive the turn in a session the agent could keep
// using.
type turnState struct {
	stream  *streamingClient
	uiTools []json.RawMessage
	caller  turnCaller

	mu       sync.Mutex
	closed   bool
	sessions []*mcp.ServerSession
}

// attachSession records an MCP session that the agent opened under this turn's
// route, so that closing the turn closes the session. It reports false when the
// turn has already ended, which can happen when the turn closes between the
// route check in ServeHTTP and the SDK creating the session; the caller then
// owns closing the session, since nothing else will.
func (t *turnState) attachSession(session *mcp.ServerSession) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return false
	}
	t.sessions = append(t.sessions, session)
	return true
}

// closeSessions marks the turn ended and closes every MCP session attached to
// it. In the stateful transport closing also removes the session from the
// handler's session map, so a later request carrying its id is refused on the
// shared mount as well as here. Each close runs on its own goroutine because
// ServerSession.Close waits for the session's in-flight requests, and the chat
// handler, which runs this from its deferred closer, must not wait on a tool
// call that a straggling agent still has open.
func (t *turnState) closeSessions() {
	t.mu.Lock()
	t.closed = true
	sessions := t.sessions
	t.sessions = nil
	t.mu.Unlock()
	for _, session := range sessions {
		go session.Close()
	}
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

// register records a new turn — the browser's SSE stream, the UI tools it
// declared and the caller behind it — under a freshly minted route id, and
// returns that id plus a closer that removes the turn and closes the MCP
// sessions opened under it. The registry mints the id (nothing outside chooses
// how a turn is keyed); the closer is meant to be deferred by the chat handler
// and is idempotent. The chat handler ignores the id today — the turn-scoped
// endpoint resolves it from the request path — but the id is what the turn's
// MCP URL is built from once that URL is announced to the sidecar.
func (s *turnRegistry) register(stream *streamingClient, uiTools []json.RawMessage, caller turnCaller) (mcpRouteID string, closer func()) {
	id := uuid.NewString()
	turn := &turnState{
		stream:  stream,
		uiTools: uiTools,
		caller:  caller,
	}
	s.mu.Lock()
	s.turns[id] = turn
	s.mu.Unlock()
	return id, func() {
		s.mu.Lock()
		delete(s.turns, id)
		s.mu.Unlock()
		turn.closeSessions()
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
