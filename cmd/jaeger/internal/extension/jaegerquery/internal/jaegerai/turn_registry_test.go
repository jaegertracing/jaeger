// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testStreamingClient() *streamingClient {
	return newStreamingClient(context.Background(), httptest.NewRecorder(), "thread", "run")
}

func TestTurnRegistryRegisterGetClose(t *testing.T) {
	s := newTurnRegistry()
	client := testStreamingClient()

	assert.Nil(t, s.get("missing"), "unknown id returns nil")

	tools := []json.RawMessage{json.RawMessage(`{"name":"t"}`)}
	id, closeTurn := s.register(client, tools)
	require.NotEmpty(t, id, "register mints a route id")

	got := s.get(id)
	require.NotNil(t, got, "get returns the registered turn")
	assert.Same(t, client, got.stream, "turn carries the registered stream")
	assert.Equal(t, tools, got.uiTools, "turn carries the registered UI tools")

	closeTurn()
	assert.Nil(t, s.get(id), "get returns nil after the closer runs")
	require.NotPanics(t, closeTurn, "the closer is idempotent")
}

func TestTurnRegistryConcurrent(t *testing.T) {
	s := newTurnRegistry()
	const n = 50
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			id, closeTurn := s.register(testStreamingClient(), nil)
			s.get(id)
			closeTurn()
		})
	}
	wg.Wait()
	assert.Equal(t, 0, s.count(), "every registered turn was closed; nothing left behind")
}

// registerTurn is a test helper: it registers a turn and returns the minted route
// id, which integration tests need to dial that turn's URL. It drops the closer —
// the registry is a local test object, so the entry simply lives for the test.
func registerTurn(r *turnRegistry, stream *streamingClient, uiTools []json.RawMessage) string {
	id, _ := r.register(stream, uiTools)
	return id
}
