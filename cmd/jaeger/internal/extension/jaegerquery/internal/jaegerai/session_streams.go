// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"encoding/json"
	"sync"
)

// session holds the per-turn state the session-scoped MCP endpoint needs: the
// live SSE stream back to the browser, and the UI tools the frontend declared
// for that turn. The endpoint advertises those UI tools (alongside the built-in
// telemetry tools) and dispatches their calls onto the stream.
type session struct {
	stream  *streamingClient
	uiTools []json.RawMessage
}

// sessionStreams maps a per-turn session id to its session state. The chat
// handler mints a session id when it opens a turn, registers the stream and the
// turn's UI tools here, and removes the entry when the turn ends. The
// session-scoped MCP endpoint (`/api/ai/mcp/<sessionId>/`) looks the id up to
// confirm it belongs to an active turn and to build that turn's tool set.
//
// It is the join key between the two halves of a session: the browser's SSE
// stream (held by the chat handler) and the sidecar's MCP connection (dialed at
// the session-scoped URL).
//
// The registry is in-memory and tied to the process lifetime — sessions are
// scoped to a single HTTP chat request and the streaming client wraps a live
// http.ResponseWriter, so there is nothing meaningful to persist.
//
// Methods are safe for concurrent use: the chat handler writes from the request
// goroutine while the MCP endpoint reads from per-request goroutines. The map is
// small (one entry per active chat) so a plain RWMutex is ample.
type sessionStreams struct {
	mu       sync.RWMutex
	sessions map[string]*session
}

// newSessionStreams returns an empty registry.
func newSessionStreams() *sessionStreams {
	return &sessionStreams{
		sessions: make(map[string]*session),
	}
}

// set records the stream and UI tools for sessionID. No-op on empty id or nil
// stream. Session ids are never reused across turns, so an overwrite indicates
// a caller bug rather than a normal interleaving.
func (s *sessionStreams) set(sessionID string, stream *streamingClient, uiTools []json.RawMessage) {
	if sessionID == "" || stream == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = &session{stream: stream, uiTools: uiTools}
}

// get returns the session for sessionID, or nil when no turn is active for it.
// Returning nil (rather than an error) keeps the endpoint's call site cheap: an
// unknown or expired id is a "not an active session" case, which the endpoint
// maps to 404.
func (s *sessionStreams) get(sessionID string) *session {
	if sessionID == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[sessionID]
}

// delete removes sessionID. Idempotent: it runs from the chat handler's defer,
// so it must not panic if set never ran (e.g. session/new failed).
func (s *sessionStreams) delete(sessionID string) {
	if sessionID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}

// count returns the number of active sessions. Used by tests to observe the
// register/deregister lifecycle without reaching into the guarded map.
func (s *sessionStreams) count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}
