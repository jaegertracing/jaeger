// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetForContextualMCPIDStoresPerIDSnapshots(t *testing.T) {
	store := NewContextualToolsStore()

	toolsA := []json.RawMessage{json.RawMessage(`{"name":"tool-a"}`)}
	toolsB := []json.RawMessage{json.RawMessage(`{"name":"tool-b"}`)}

	store.SetForContextualMCPID("id-1", toolsA)
	store.SetForContextualMCPID("id-2", toolsB)

	gotA := store.GetContextualToolsForID("id-1")
	assert.Len(t, gotA, 1)
	assert.Equal(t, "tool-a", gotA[0].(map[string]any)["name"],
		"GetContextualToolsForID must return the snapshot for the requested id, not the most recent one")

	gotB := store.GetContextualToolsForID("id-2")
	assert.Len(t, gotB, 1)
	assert.Equal(t, "tool-b", gotB[0].(map[string]any)["name"])
}

func TestSetForContextualMCPIDSkipsInvalidJSON(t *testing.T) {
	store := NewContextualToolsStore()

	store.SetForContextualMCPID("id-1", []json.RawMessage{
		json.RawMessage(`{"name":"valid"}`),
		json.RawMessage(`{broken`),
	})

	got := store.GetContextualToolsForID("id-1")
	assert.Len(t, got, 1, "invalid JSON entries must be skipped silently")
	assert.Equal(t, "valid", got[0].(map[string]any)["name"])
}

func TestGetContextualToolsForIDReturnsNilWhenUnknown(t *testing.T) {
	store := NewContextualToolsStore()
	assert.Nil(t, store.GetContextualToolsForID("id-1"), "unknown id → nil snapshot")

	store.SetForContextualMCPID("id-1", nil)
	assert.Nil(t, store.GetContextualToolsForID("id-1"),
		"id with empty tools list should still surface as nil to callers")
}

func TestGetContextualToolsForIDReturnsNilForEmptyID(t *testing.T) {
	store := NewContextualToolsStore()
	store.SetForContextualMCPID("id-1", []json.RawMessage{json.RawMessage(`{"name":"tool-a"}`)})

	assert.Nil(t, store.GetContextualToolsForID(""),
		"empty contextual_mcp_id must never match — guards against misrouted snapshots")
}

func TestSetForContextualMCPIDIgnoresEmptyID(t *testing.T) {
	store := NewContextualToolsStore()

	// Writes under "" must be no-ops; otherwise we'd leak entries that
	// Get/Delete refuse to address (they both short-circuit on "").
	store.SetForContextualMCPID("", []json.RawMessage{json.RawMessage(`{"name":"orphan"}`)})

	store.mu.RLock()
	_, present := store.byID[""]
	store.mu.RUnlock()
	assert.False(t, present, `SetForContextualMCPID("", ...) must not create an entry under ""`)
}

func TestDeleteForContextualMCPIDRemovesSnapshot(t *testing.T) {
	store := NewContextualToolsStore()
	store.SetForContextualMCPID("id-1", []json.RawMessage{json.RawMessage(`{"name":"tool-a"}`)})
	store.SetForContextualMCPID("id-2", []json.RawMessage{json.RawMessage(`{"name":"tool-b"}`)})

	store.DeleteForContextualMCPID("id-1")

	assert.Nil(t, store.GetContextualToolsForID("id-1"),
		"deleted id must no longer resolve")
	assert.NotNil(t, store.GetContextualToolsForID("id-2"),
		"DeleteForContextualMCPID must only affect the target id")

	// Deleting an unknown or empty id must not panic or affect state.
	store.DeleteForContextualMCPID("nonexistent")
	store.DeleteForContextualMCPID("")
	assert.NotNil(t, store.GetContextualToolsForID("id-2"))
}

func TestGetContextualToolsForIDReturnsDefensiveCopy(t *testing.T) {
	store := NewContextualToolsStore()
	store.SetForContextualMCPID("id-1", []json.RawMessage{json.RawMessage(`{"name":"tool-a"}`)})

	first := store.GetContextualToolsForID("id-1")
	first[0] = "mutated"

	second := store.GetContextualToolsForID("id-1")
	assert.Equal(t, "tool-a", second[0].(map[string]any)["name"],
		"callers mutating their copy must not corrupt the stored snapshot")
}

func TestGetContextualToolsForIDIsolatesNestedMutations(t *testing.T) {
	store := NewContextualToolsStore()
	store.SetForContextualMCPID("id-1", []json.RawMessage{json.RawMessage(`{"name":"tool-a"}`)})

	first := store.GetContextualToolsForID("id-1")
	first[0].(map[string]any)["name"] = "hijacked"

	second := store.GetContextualToolsForID("id-1")
	assert.Equal(t, "tool-a", second[0].(map[string]any)["name"],
		"mutating a nested map in the returned snapshot must not corrupt the stored entry")
}

func TestGetContextualToolsForIDSkipsUnparsableEntries(t *testing.T) {
	store := NewContextualToolsStore()
	store.SetForContextualMCPID("id-1", []json.RawMessage{json.RawMessage(`{"name":"tool-a"}`)})

	// White-box: simulate a corrupted entry slipping past the write-side
	// validation (e.g. memory corruption) and verify the read path degrades
	// gracefully instead of exploding.
	store.mu.Lock()
	store.byID["id-1"] = append(store.byID["id-1"], json.RawMessage(`{broken`))
	store.mu.Unlock()

	got := store.GetContextualToolsForID("id-1")
	assert.Len(t, got, 1, "unparsable raw entries must be skipped at read time")
	assert.Equal(t, "tool-a", got[0].(map[string]any)["name"])

	// All entries corrupted → read returns nil.
	store.mu.Lock()
	store.byID["id-1"] = []json.RawMessage{json.RawMessage(`{also-broken`)}
	store.mu.Unlock()
	assert.Nil(t, store.GetContextualToolsForID("id-1"))
}

func TestSetForContextualMCPIDCopiesRawBytes(t *testing.T) {
	store := NewContextualToolsStore()
	raw := json.RawMessage(`{"name":"tool-a"}`)
	store.SetForContextualMCPID("id-1", []json.RawMessage{raw})

	// Mutate the caller's bytes after storing. The snapshot must be unaffected.
	for i := range raw {
		raw[i] = 'x'
	}

	got := store.GetContextualToolsForID("id-1")
	assert.Equal(t, "tool-a", got[0].(map[string]any)["name"],
		"SetForContextualMCPID must defensively copy raw bytes")
}
