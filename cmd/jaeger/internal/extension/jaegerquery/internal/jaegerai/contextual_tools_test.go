// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"encoding/json"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetForSessionStoresPerSessionSnapshots(t *testing.T) {
	store := NewContextualToolsStore()

	toolsA := []json.RawMessage{json.RawMessage(`{"name":"tool-a"}`)}
	toolsB := []json.RawMessage{json.RawMessage(`{"name":"tool-b"}`)}

	store.SetForSession("sess-1", toolsA)
	store.SetForSession("sess-2", toolsB)

	gotA := store.GetContextualToolsForSession("sess-1")
	assert.Len(t, gotA, 1)
	assert.Equal(t, "tool-a", gotA[0].(map[string]any)["name"],
		"GetContextualToolsForSession must return the snapshot for the requested session, not the most recent one")

	gotB := store.GetContextualToolsForSession("sess-2")
	assert.Len(t, gotB, 1)
	assert.Equal(t, "tool-b", gotB[0].(map[string]any)["name"])
}

func TestSetForSessionSkipsInvalidJSON(t *testing.T) {
	store := NewContextualToolsStore()

	store.SetForSession("sess-1", []json.RawMessage{
		json.RawMessage(`{"name":"valid"}`),
		json.RawMessage(`{broken`),
	})

	got := store.GetContextualToolsForSession("sess-1")
	assert.Len(t, got, 1, "invalid JSON entries must be skipped silently")
	assert.Equal(t, "valid", got[0].(map[string]any)["name"])
}

func TestGetContextualToolsForSessionReturnsNilWhenUnknown(t *testing.T) {
	store := NewContextualToolsStore()
	assert.Nil(t, store.GetContextualToolsForSession("sess-1"), "unknown session → nil snapshot")

	store.SetForSession("sess-1", nil)
	assert.Nil(t, store.GetContextualToolsForSession("sess-1"),
		"session with empty tools list should still surface as nil to callers")

	// Storing a nil/empty list must not leave an unreachable map entry —
	// otherwise the store accumulates keys that Get reports as "no
	// snapshot" and only DeleteForSession can remove.
	store.mu.RLock()
	_, present := store.bySession["sess-1"]
	store.mu.RUnlock()
	assert.False(t, present, "empty validated tools list must not leave a map entry")
}

func TestSetForSessionEmptyValidatedListClearsExisting(t *testing.T) {
	// Writing a snapshot followed by an all-invalid (or nil) snapshot
	// must clear the entry, so a subsequent caller sees the store as if
	// no tools were ever registered for that session.
	store := NewContextualToolsStore()
	store.SetForSession("sess-1", []json.RawMessage{json.RawMessage(`{"name":"tool-a"}`)})

	store.mu.RLock()
	_, presentBefore := store.bySession["sess-1"]
	store.mu.RUnlock()
	require.True(t, presentBefore, "precondition: sess-1 should be in the map after the first set")

	// All entries invalid → validated list ends up empty → delete the entry.
	store.SetForSession("sess-1", []json.RawMessage{json.RawMessage(`{broken`)})

	store.mu.RLock()
	_, presentAfter := store.bySession["sess-1"]
	store.mu.RUnlock()
	assert.False(t, presentAfter, "all-invalid snapshot must remove the existing entry")
	assert.Nil(t, store.GetContextualToolsForSession("sess-1"))
}

func TestGetContextualToolsForSessionReturnsNilForEmptySession(t *testing.T) {
	store := NewContextualToolsStore()
	store.SetForSession("sess-1", []json.RawMessage{json.RawMessage(`{"name":"tool-a"}`)})

	assert.Nil(t, store.GetContextualToolsForSession(""),
		"empty session id must never match — guards against misrouted snapshots")
}

func TestSetForSessionIgnoresEmptySession(t *testing.T) {
	store := NewContextualToolsStore()

	// Writes under "" must be no-ops; otherwise we'd leak entries that
	// Get/Delete refuse to address (they both short-circuit on "").
	store.SetForSession("", []json.RawMessage{json.RawMessage(`{"name":"orphan"}`)})

	store.mu.RLock()
	_, present := store.bySession[""]
	store.mu.RUnlock()
	assert.False(t, present, `SetForSession("", ...) must not create an entry under ""`)
}

func TestDeleteForSessionRemovesSnapshot(t *testing.T) {
	store := NewContextualToolsStore()
	store.SetForSession("sess-1", []json.RawMessage{json.RawMessage(`{"name":"tool-a"}`)})
	store.SetForSession("sess-2", []json.RawMessage{json.RawMessage(`{"name":"tool-b"}`)})

	store.DeleteForSession("sess-1")

	assert.Nil(t, store.GetContextualToolsForSession("sess-1"),
		"deleted session must no longer resolve")
	assert.NotNil(t, store.GetContextualToolsForSession("sess-2"),
		"DeleteForSession must only affect the target session")

	// Deleting an unknown or empty session must not panic or affect state.
	store.DeleteForSession("nonexistent")
	store.DeleteForSession("")
	assert.NotNil(t, store.GetContextualToolsForSession("sess-2"))
}

func TestGetContextualToolsForSessionReturnsDefensiveCopy(t *testing.T) {
	store := NewContextualToolsStore()
	store.SetForSession("sess-1", []json.RawMessage{json.RawMessage(`{"name":"tool-a"}`)})

	first := store.GetContextualToolsForSession("sess-1")
	first[0] = "mutated"

	second := store.GetContextualToolsForSession("sess-1")
	assert.Equal(t, "tool-a", second[0].(map[string]any)["name"],
		"callers mutating their copy must not corrupt the stored snapshot")
}

func TestGetContextualToolsForSessionIsolatesNestedMutations(t *testing.T) {
	store := NewContextualToolsStore()
	store.SetForSession("sess-1", []json.RawMessage{json.RawMessage(`{"name":"tool-a"}`)})

	first := store.GetContextualToolsForSession("sess-1")
	first[0].(map[string]any)["name"] = "hijacked"

	second := store.GetContextualToolsForSession("sess-1")
	assert.Equal(t, "tool-a", second[0].(map[string]any)["name"],
		"mutating a nested map in the returned snapshot must not corrupt the stored entry")
}

func TestGetContextualToolsForSessionSkipsUnparsableEntries(t *testing.T) {
	store := NewContextualToolsStore()
	store.SetForSession("sess-1", []json.RawMessage{json.RawMessage(`{"name":"tool-a"}`)})

	// White-box: simulate a corrupted entry slipping past the write-side
	// validation (e.g. memory corruption) and verify the read path degrades
	// gracefully instead of exploding.
	store.mu.Lock()
	store.bySession["sess-1"] = append(store.bySession["sess-1"], json.RawMessage(`{broken`))
	store.mu.Unlock()

	got := store.GetContextualToolsForSession("sess-1")
	assert.Len(t, got, 1, "unparsable raw entries must be skipped at read time")
	assert.Equal(t, "tool-a", got[0].(map[string]any)["name"])

	// All entries corrupted → read returns nil.
	store.mu.Lock()
	store.bySession["sess-1"] = []json.RawMessage{json.RawMessage(`{also-broken`)}
	store.mu.Unlock()
	assert.Nil(t, store.GetContextualToolsForSession("sess-1"))
}

func TestSetForSessionCopiesRawBytes(t *testing.T) {
	store := NewContextualToolsStore()
	raw := json.RawMessage(`{"name":"tool-a"}`)
	store.SetForSession("sess-1", []json.RawMessage{raw})

	// Mutate the caller's bytes after storing. The snapshot must be unaffected.
	for i := range raw {
		raw[i] = 'x'
	}

	got := store.GetContextualToolsForSession("sess-1")
	assert.Equal(t, "tool-a", got[0].(map[string]any)["name"],
		"SetForSession must defensively copy raw bytes")
}

// TestContextualToolsStoreConcurrentAccess exercises the RWMutex with a
// fan-out of writers, readers, and deleters touching distinct sessions.
// Run under -race to catch any unsynchronised access. The test does not
// assert on values — concurrent map access is itself the property under
// test; the assertions verify the store stays usable afterwards.
func TestContextualToolsStoreConcurrentAccess(t *testing.T) {
	store := NewContextualToolsStore()

	const goroutines = 32
	const iterations = 200

	var wg sync.WaitGroup
	wg.Add(goroutines * 3)

	for w := 0; w < goroutines; w++ {
		go func(w int) {
			defer wg.Done()
			id := "writer-" + strconv.Itoa(w)
			for i := 0; i < iterations; i++ {
				store.SetForSession(id, []json.RawMessage{
					json.RawMessage(`{"name":"tool-` + strconv.Itoa(i) + `"}`),
				})
			}
		}(w)
	}

	for r := 0; r < goroutines; r++ {
		go func(r int) {
			defer wg.Done()
			id := "writer-" + strconv.Itoa(r)
			for i := 0; i < iterations; i++ {
				_ = store.GetContextualToolsForSession(id)
			}
		}(r)
	}

	for d := 0; d < goroutines; d++ {
		go func(d int) {
			defer wg.Done()
			id := "deleter-" + strconv.Itoa(d)
			for i := 0; i < iterations; i++ {
				store.SetForSession(id, []json.RawMessage{json.RawMessage(`{"name":"tmp"}`)})
				store.DeleteForSession(id)
			}
		}(d)
	}

	wg.Wait()

	// Sanity: store is still usable after the storm.
	store.SetForSession("after", []json.RawMessage{json.RawMessage(`{"name":"final"}`)})
	got := store.GetContextualToolsForSession("after")
	assert.Len(t, got, 1)
	assert.Equal(t, "final", got[0].(map[string]any)["name"])
}
