// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"sync"
	"time"

	"github.com/coder/acp-go-sdk"
)

type sessionEntry struct {
	sessionID  acp.SessionId
	lastActive time.Time
}

// sessionRegistry maps an AG-UI threadId to its active ACP sessionId.
// This allows subsequent chat turns on the same thread to resume the
// warm ACP session rather than creating a fresh session on every request.
//
// Like turnRegistry, the registry is in-memory and tied to the process
// lifetime. It evicts sessions after an idle TTL.
type sessionRegistry struct {
	mu       sync.RWMutex
	sessions map[string]sessionEntry
	ttl      time.Duration
}

// newSessionRegistry constructs a session registry with the given idle TTL.
// A non-positive TTL defaults to 30 minutes.
func newSessionRegistry(ttl time.Duration) *sessionRegistry {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &sessionRegistry{
		sessions: make(map[string]sessionEntry),
		ttl:      ttl,
	}
}

// get returns the active ACP session id for threadID, refreshing its idle TTL.
// Returns "", false if threadID is empty, not found, or expired.
func (r *sessionRegistry) get(threadID string) (acp.SessionId, bool) {
	if threadID == "" {
		return "", false
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.sessions[threadID]
	if !ok {
		return "", false
	}
	if time.Since(entry.lastActive) > r.ttl {
		delete(r.sessions, threadID)
		return "", false
	}
	entry.lastActive = time.Now()
	r.sessions[threadID] = entry
	return entry.sessionID, true
}

// set records the session id for threadID with the current timestamp.
func (r *sessionRegistry) set(threadID string, sessionID acp.SessionId) {
	if threadID == "" || sessionID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[threadID] = sessionEntry{
		sessionID:  sessionID,
		lastActive: time.Now(),
	}
}

// delete removes the session entry for threadID.
func (r *sessionRegistry) delete(threadID string) {
	if threadID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, threadID)
}

// count returns the active number of tracked thread sessions.
func (r *sessionRegistry) count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.sessions)
}
