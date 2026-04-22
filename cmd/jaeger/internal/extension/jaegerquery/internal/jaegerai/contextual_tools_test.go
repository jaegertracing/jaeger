// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetForSessionStoresPerSessionSnapshots(t *testing.T) {
	store := NewContextualToolsStore()

	toolsA := []json.RawMessage{json.RawMessage(`{"name":"tool-a"}`)}
	toolsB := []json.RawMessage{json.RawMessage(`{"name":"tool-b"}`)}

	store.SetForSession("session-1", toolsA)
	store.SetForSession("session-2", toolsB)

	gotA := store.GetContextualToolsForSession("session-1")
	assert.Len(t, gotA, 1)
	assert.Equal(t, "tool-a", gotA[0].(map[string]any)["name"],
		"GetContextualToolsForSession must return the snapshot for the requested session, not the most recent one")

	gotB := store.GetContextualToolsForSession("session-2")
	assert.Len(t, gotB, 1)
	assert.Equal(t, "tool-b", gotB[0].(map[string]any)["name"])
}

func TestSetForSessionSkipsInvalidJSON(t *testing.T) {
	store := NewContextualToolsStore()

	store.SetForSession("session-1", []json.RawMessage{
		json.RawMessage(`{"name":"valid"}`),
		json.RawMessage(`{broken`),
	})

	got := store.GetContextualToolsForSession("session-1")
	assert.Len(t, got, 1, "invalid JSON entries must be skipped silently")
	assert.Equal(t, "valid", got[0].(map[string]any)["name"])
}

func TestGetContextualToolsForSessionReturnsNilWhenUnknown(t *testing.T) {
	store := NewContextualToolsStore()
	assert.Nil(t, store.GetContextualToolsForSession("session-1"), "unknown session → nil snapshot")

	store.SetForSession("session-1", nil)
	assert.Nil(t, store.GetContextualToolsForSession("session-1"),
		"session with empty tools list should still surface as nil to callers")
}

func TestGetContextualToolsForSessionReturnsNilForEmptySessionID(t *testing.T) {
	store := NewContextualToolsStore()
	store.SetForSession("session-1", []json.RawMessage{json.RawMessage(`{"name":"tool-a"}`)})

	assert.Nil(t, store.GetContextualToolsForSession(""),
		"empty session_id must never match — guards against misrouted snapshots")
}

func TestDeleteForSessionRemovesSnapshot(t *testing.T) {
	store := NewContextualToolsStore()
	store.SetForSession("session-1", []json.RawMessage{json.RawMessage(`{"name":"tool-a"}`)})
	store.SetForSession("session-2", []json.RawMessage{json.RawMessage(`{"name":"tool-b"}`)})

	store.DeleteForSession("session-1")

	assert.Nil(t, store.GetContextualToolsForSession("session-1"),
		"deleted session must no longer resolve")
	assert.NotNil(t, store.GetContextualToolsForSession("session-2"),
		"DeleteForSession must only affect the target session")

	// Deleting an unknown or empty session must not panic or affect state.
	store.DeleteForSession("nonexistent")
	store.DeleteForSession("")
	assert.NotNil(t, store.GetContextualToolsForSession("session-2"))
}

func TestGetContextualToolsForSessionReturnsDefensiveCopy(t *testing.T) {
	store := NewContextualToolsStore()
	store.SetForSession("session-1", []json.RawMessage{json.RawMessage(`{"name":"tool-a"}`)})

	first := store.GetContextualToolsForSession("session-1")
	first[0] = "mutated"

	second := store.GetContextualToolsForSession("session-1")
	assert.Equal(t, "tool-a", second[0].(map[string]any)["name"],
		"callers mutating their copy must not corrupt the stored snapshot")
}

func TestGetContextualToolsForSessionIsolatesNestedMutations(t *testing.T) {
	store := NewContextualToolsStore()
	store.SetForSession("session-1", []json.RawMessage{json.RawMessage(`{"name":"tool-a"}`)})

	first := store.GetContextualToolsForSession("session-1")
	first[0].(map[string]any)["name"] = "hijacked"

	second := store.GetContextualToolsForSession("session-1")
	assert.Equal(t, "tool-a", second[0].(map[string]any)["name"],
		"mutating a nested map in the returned snapshot must not corrupt the stored entry")
}

func TestGetContextualToolsForSessionSkipsUnparsableEntries(t *testing.T) {
	store := NewContextualToolsStore()
	store.SetForSession("session-1", []json.RawMessage{json.RawMessage(`{"name":"tool-a"}`)})

	// White-box: simulate a corrupted entry slipping past the write-side
	// validation (e.g. memory corruption) and verify the read path degrades
	// gracefully instead of exploding.
	store.mu.Lock()
	store.bySession["session-1"] = append(store.bySession["session-1"], json.RawMessage(`{broken`))
	store.mu.Unlock()

	got := store.GetContextualToolsForSession("session-1")
	assert.Len(t, got, 1, "unparsable raw entries must be skipped at read time")
	assert.Equal(t, "tool-a", got[0].(map[string]any)["name"])

	// All entries corrupted → read returns nil.
	store.mu.Lock()
	store.bySession["session-1"] = []json.RawMessage{json.RawMessage(`{also-broken`)}
	store.mu.Unlock()
	assert.Nil(t, store.GetContextualToolsForSession("session-1"))
}

func TestSetForSessionCopiesRawBytes(t *testing.T) {
	store := NewContextualToolsStore()
	raw := json.RawMessage(`{"name":"tool-a"}`)
	store.SetForSession("session-1", []json.RawMessage{raw})

	// Mutate the caller's bytes after storing. The snapshot must be unaffected.
	for i := range raw {
		raw[i] = 'x'
	}

	got := store.GetContextualToolsForSession("session-1")
	assert.Equal(t, "tool-a", got[0].(map[string]any)["name"],
		"SetForSession must defensively copy raw bytes")
}