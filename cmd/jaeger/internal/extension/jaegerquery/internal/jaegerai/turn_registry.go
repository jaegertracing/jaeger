// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"sync"

	"github.com/google/uuid"
)

// turnTokenHeader is the request header that carries a turn's token.
const turnTokenHeader = "X-Jaeger-Turn-Token" //nolint:gosec // a header name, not a credential

// turnState holds what the turn-scoped MCP endpoint needs for one turn: the AG-UI
// run that writes to the browser and the UI tools the frontend declared.
type turnState struct {
	run     *aguiRun
	uiTools []json.RawMessage
	// tokenHash is the SHA-256 of the turn's token; see turnGrant.token.
	tokenHash [sha256.Size]byte
}

// authorize reports whether token is this turn's token. The comparison is on
// hashes and in constant time, so a wrong token learns nothing about the right
// one.
func (t *turnState) authorize(token string) bool {
	sum := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(sum[:], t.tokenHash[:]) == 1
}

// turnGrant is what the chat handler announces to the agent for one turn: the
// route id that names the turn-scoped MCP endpoint and the token that the
// endpoint requires on every request to it. The id travels in the URL and the
// token in a header, because URLs end up in access logs and traces while
// headers do not. Both are random, minted per turn and void when the turn ends.
type turnGrant struct {
	routeID string
	token   string
}

// newTurnToken returns a random token with at least 128 bits of entropy.
func newTurnToken() string {
	return rand.Text()
}

// turnRegistry maps a per-turn route id (mcpRouteID) to its turn state. The chat
// handler mints a route id when it opens a turn, registers the run and the
// turn's UI tools here, and removes the entry when the turn ends. The turn-scoped
// MCP endpoint (`/api/ai/mcp/<mcpRouteID>/`) looks the id up to confirm it belongs
// to an active turn and to build that turn's tool set.
//
// It is the join key between the two halves of a turn: the browser's SSE stream
// (held by the chat handler) and the sidecar's MCP connection (dialed at the
// turn-scoped URL).
//
// The registry is in-memory and tied to the process lifetime — turns are scoped
// to a single HTTP chat request and the AG-UI run wraps a live
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
// declared — under a freshly minted route id and token,
// and returns that grant plus a closer that removes the turn and closes its MCP
// sessions. The registry mints both (nothing outside chooses how a turn is
// keyed); the closer is meant to be deferred by the chat handler and is
// idempotent. The chat handler announces the grant to the sidecar as the turn's
// MCP URL and header.
func (s *turnRegistry) register(run *aguiRun, uiTools []json.RawMessage) (grant turnGrant, closer func()) {
	grant = turnGrant{routeID: uuid.NewString(), token: newTurnToken()}
	turn := &turnState{
		run:       run,
		uiTools:   uiTools,
		tokenHash: sha256.Sum256([]byte(grant.token)),
	}
	s.mu.Lock()
	s.turns[grant.routeID] = turn
	s.mu.Unlock()
	return grant, func() {
		s.mu.Lock()
		delete(s.turns, grant.routeID)
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
