// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStreamingClient(t *testing.T) *streamingClient {
	t.Helper()
	rec := httptest.NewRecorder()
	return newStreamingClient(t.Context(), rec, "thread-x", "run-x")
}

func TestSessionStreamsSetGetDeleteRoundTrip(t *testing.T) {
	streams := NewSessionStreams()
	client := newTestStreamingClient(t)

	require.Nil(t, streams.Get("missing"), "Get on unknown id returns nil")

	streams.Set("sess-1", client)
	assert.Same(t, client, streams.Get("sess-1"))

	streams.Delete("sess-1")
	assert.Nil(t, streams.Get("sess-1"),
		"Delete must remove the entry so the MCP proxy stops dispatching after turn end")
}

func TestSessionStreamsRejectsEmptyOrNilInputs(t *testing.T) {
	// Defensive: callers (chat handler) might construct an empty session
	// id when ACP session/new returns one, or pass a nil streaming client
	// from a code path that errored before the client was built. Either
	// should be a no-op so the registry never accumulates garbage entries
	// that nothing can reach via Get.
	streams := NewSessionStreams()
	client := newTestStreamingClient(t)

	streams.Set("", client)
	streams.Set("sess-x", nil)

	assert.Nil(t, streams.Get(""), "empty session id must not be stored")
	assert.Nil(t, streams.Get("sess-x"), "nil client must not be stored")
}

func TestSessionStreamsDeleteIsIdempotent(_ *testing.T) {
	// The chat handler's defer runs even if Set was never called (e.g.
	// session/new failed before reaching the registration point). Delete
	// must not panic in that case — the entire point of having the defer
	// unconditional is to keep cleanup simple. No assertion needed; the
	// test passes if these calls return without panic.
	streams := NewSessionStreams()
	streams.Delete("never-set")
	streams.Delete("")
}

func TestSessionStreamsOverwriteReplacesEntry(t *testing.T) {
	// The gateway never reuses session ids in normal flow, so a second
	// Set is technically a bug — but if it happens, the latter must win
	// so a stale streaming client (whose ResponseWriter may already be
	// closed) can't be returned to the MCP proxy.
	streams := NewSessionStreams()
	first := newTestStreamingClient(t)
	second := newTestStreamingClient(t)

	streams.Set("sess-1", first)
	streams.Set("sess-1", second)

	assert.Same(t, second, streams.Get("sess-1"))
}

func TestSessionStreamsIsRaceFree(t *testing.T) {
	// SessionStreams is read from arbitrary HTTP goroutines (MCP proxy)
	// while the chat handler's goroutine writes. Run the operations in
	// parallel and rely on -race to catch any unguarded access. We
	// don't assert specific final state because interleavings can vary;
	// the only failure mode we're chasing is a data race detection.
	streams := NewSessionStreams()
	client := newTestStreamingClient(t)

	const iterations = 200
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := 0; i < iterations; i++ {
			streams.Set("sess-race", client)
		}
	})
	wg.Go(func() {
		for i := 0; i < iterations; i++ {
			_ = streams.Get("sess-race")
		}
	})
	wg.Go(func() {
		for i := 0; i < iterations; i++ {
			streams.Delete("sess-race")
		}
	})
	wg.Wait()
}
