// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"testing"
	"time"

	"github.com/coder/acp-go-sdk"
	"github.com/stretchr/testify/assert"
)

func TestSessionRegistry_BasicLifecycle(t *testing.T) {
	reg := newSessionRegistry(5 * time.Minute)
	assert.Equal(t, 0, reg.count())

	// Empty lookups
	sid, ok := reg.get("")
	assert.False(t, ok)
	assert.Empty(t, sid)

	sid, ok = reg.get("non-existent")
	assert.False(t, ok)
	assert.Empty(t, sid)

	// Set and get
	reg.set("thread-1", acp.SessionId("sess-1"))
	assert.Equal(t, 1, reg.count())

	sid, ok = reg.get("thread-1")
	assert.True(t, ok)
	assert.Equal(t, acp.SessionId("sess-1"), sid)

	// Update existing
	reg.set("thread-1", acp.SessionId("sess-2"))
	sid, ok = reg.get("thread-1")
	assert.True(t, ok)
	assert.Equal(t, acp.SessionId("sess-2"), sid)

	// Delete
	reg.delete("thread-1")
	assert.Equal(t, 0, reg.count())
	sid, ok = reg.get("thread-1")
	assert.False(t, ok)
	assert.Empty(t, sid)

	// Empty set / delete no-ops
	reg.set("", acp.SessionId("sess-x"))
	reg.set("thread-x", "")
	assert.Equal(t, 0, reg.count())
	reg.delete("")
}

func TestSessionRegistry_TTLExpiration(t *testing.T) {
	reg := newSessionRegistry(10 * time.Millisecond)

	reg.set("thread-short", acp.SessionId("sess-short"))
	sid, ok := reg.get("thread-short")
	assert.True(t, ok)
	assert.Equal(t, acp.SessionId("sess-short"), sid)

	time.Sleep(25 * time.Millisecond)

	sid, ok = reg.get("thread-short")
	assert.False(t, ok)
	assert.Empty(t, sid)
	assert.Equal(t, 0, reg.count())
}
