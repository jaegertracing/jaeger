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

func TestGetContextualToolsForSessionReturnsDefensiveCopy(t *testing.T) {
	store := NewContextualToolsStore()
	store.SetForSession("session-1", []json.RawMessage{json.RawMessage(`{"name":"tool-a"}`)})

	first := store.GetContextualToolsForSession("session-1")
	first[0] = "mutated"

	second := store.GetContextualToolsForSession("session-1")
	assert.Equal(t, "tool-a", second[0].(map[string]any)["name"],
		"callers mutating their copy must not corrupt the stored snapshot")
}
