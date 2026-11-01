// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testAGUIRun() *aguiRun {
	return newAGUIRun(context.Background(), httptest.NewRecorder(), "thread", "run")
}

func TestTurnRegistryRegisterGetClose(t *testing.T) {
	s := newTurnRegistry()
	client := testAGUIRun()

	assert.Nil(t, s.get("missing"), "unknown id returns nil")

	tools := []json.RawMessage{json.RawMessage(`{"name":"t"}`)}
	caller := turnCaller{bearerToken: "tok"}
	grant, closeTurn := s.register(client, tools, caller)
	require.NotEmpty(t, grant.routeID, "register mints a route id")
	require.NotEmpty(t, grant.token, "register mints a token")

	got := s.get(grant.routeID)
	require.NotNil(t, got, "get returns the registered turn")
	assert.Same(t, client, got.run, "turn carries the registered AG-UI run")
	assert.Equal(t, tools, got.uiTools, "turn carries the registered UI tools")
	assert.Equal(t, caller, got.caller, "turn carries the chat caller")
	assert.True(t, got.authorize(grant.token), "the turn accepts its own token")
	assert.False(t, got.authorize(""), "the turn refuses an empty token")
	assert.False(t, got.authorize(grant.token+"x"), "the turn refuses a different token")

	other, closeOther := s.register(client, nil, turnCaller{})
	defer closeOther()
	assert.NotEqual(t, grant.token, other.token, "every turn gets its own token")
	assert.False(t, got.authorize(other.token), "another turn's token does not open this turn")

	closeTurn()
	assert.Nil(t, s.get(grant.routeID), "get returns nil after the closer runs")
	require.NotPanics(t, closeTurn, "the closer is idempotent")
}

func TestTurnRegistryConcurrent(t *testing.T) {
	s := newTurnRegistry()
	const n = 50
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			grant, closeTurn := s.register(testAGUIRun(), nil, turnCaller{})
			s.get(grant.routeID)
			closeTurn()
		})
	}
	wg.Wait()
	assert.Equal(t, 0, s.count(), "every registered turn was closed; nothing left behind")
}

// registerTurn is a test helper: it registers a turn and returns the minted route
// id, which integration tests need to dial that turn's URL. It drops the closer —
// the registry is a local test object, so the entry simply lives for the test.
func registerTurn(r *turnRegistry, run *aguiRun, uiTools []json.RawMessage) turnGrant {
	grant, _ := r.register(stream, uiTools, turnCaller{})
	return grant
}

// A session that reaches a turn after the turn ended must not be adopted, or
// nothing would ever close it; attachSession says so and the turn stays empty.
func TestTurnStateRefusesSessionAfterClose(t *testing.T) {
	turn := &turnState{}
	turn.closeSessions()
	assert.False(t, turn.attachSession(&mcp.ServerSession{}), "a closed turn adopts no session")
	assert.Empty(t, turn.sessions)
}
