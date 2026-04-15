// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package querysvc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetContextualToolsForSessionStoresPerSessionSnapshots(t *testing.T) {
	qs := &QueryService{contextualTools: newContextualToolsState()}

	toolsA := []json.RawMessage{json.RawMessage(`{"name":"tool-a"}`)}
	toolsB := []json.RawMessage{json.RawMessage(`{"name":"tool-b"}`)}

	qs.SetContextualToolsForSession("session-1", toolsA)
	qs.SetContextualToolsForSession("session-2", toolsB)

	gotA := qs.GetContextualToolsForSession("session-1")
	assert.Len(t, gotA, 1)
	assert.Equal(t, "tool-a", gotA[0].(map[string]any)["name"],
		"GetContextualToolsForSession must return the snapshot for the requested session, not the most recent one")

	gotB := qs.GetContextualToolsForSession("session-2")
	assert.Len(t, gotB, 1)
	assert.Equal(t, "tool-b", gotB[0].(map[string]any)["name"])
}

func TestSetContextualToolsForSessionSkipsInvalidJSON(t *testing.T) {
	qs := &QueryService{contextualTools: newContextualToolsState()}

	qs.SetContextualToolsForSession("session-1", []json.RawMessage{
		json.RawMessage(`{"name":"valid"}`),
		json.RawMessage(`{broken`),
	})

	got := qs.GetContextualToolsForSession("session-1")
	assert.Len(t, got, 1, "invalid JSON entries must be skipped silently")
	assert.Equal(t, "valid", got[0].(map[string]any)["name"])
}

func TestGetContextualToolsForSessionReturnsNilWhenUnknown(t *testing.T) {
	qs := &QueryService{contextualTools: newContextualToolsState()}
	assert.Nil(t, qs.GetContextualToolsForSession("session-1"), "unknown session → nil snapshot")

	qs.SetContextualToolsForSession("session-1", nil)
	assert.Nil(t, qs.GetContextualToolsForSession("session-1"),
		"session with empty tools list should still surface as nil to callers")
}

func TestGetContextualToolsForSessionReturnsNilForEmptySessionID(t *testing.T) {
	qs := &QueryService{contextualTools: newContextualToolsState()}
	qs.SetContextualToolsForSession("session-1", []json.RawMessage{json.RawMessage(`{"name":"tool-a"}`)})

	assert.Nil(t, qs.GetContextualToolsForSession(""),
		"empty session_id must never match — guards against misrouted snapshots")
}

func TestGetContextualToolsForSessionReturnsDefensiveCopy(t *testing.T) {
	qs := &QueryService{contextualTools: newContextualToolsState()}
	qs.SetContextualToolsForSession("session-1", []json.RawMessage{json.RawMessage(`{"name":"tool-a"}`)})

	first := qs.GetContextualToolsForSession("session-1")
	first[0] = "mutated"

	second := qs.GetContextualToolsForSession("session-1")
	assert.Equal(t, "tool-a", second[0].(map[string]any)["name"],
		"callers mutating their copy must not corrupt the stored snapshot")
}

func TestContextualToolsNilReceiverIsSafe(t *testing.T) {
	var qs *QueryService
	qs.SetContextualToolsForSession("session-1", []json.RawMessage{json.RawMessage(`{"name":"tool-a"}`)})
	assert.Nil(t, qs.GetContextualToolsForSession("session-1"))
}
