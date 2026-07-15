// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerai

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testStreamingClient() *streamingClient {
	return newStreamingClient(context.Background(), httptest.NewRecorder(), "thread", "run")
}

func TestSessionStreamsSetGetDelete(t *testing.T) {
	s := newSessionStreams()
	client := testStreamingClient()

	assert.Nil(t, s.get("missing"), "unknown id returns nil")

	tools := []json.RawMessage{json.RawMessage(`{"name":"t"}`)}
	s.set("sess-1", client, tools)
	got := s.get("sess-1")
	require.NotNil(t, got, "get returns the registered session")
	assert.Same(t, client, got.stream, "session carries the registered stream")
	assert.Equal(t, tools, got.uiTools, "session carries the registered UI tools")

	s.delete("sess-1")
	assert.Nil(t, s.get("sess-1"), "get returns nil after delete")
}

func TestSessionStreamsIgnoresEmptyOrNil(t *testing.T) {
	s := newSessionStreams()

	s.set("", testStreamingClient(), nil) // empty id: no-op
	s.set("sess", nil, nil)               // nil client: no-op
	assert.Nil(t, s.get(""))
	assert.Nil(t, s.get("sess"))

	// delete is idempotent and safe when set never ran.
	require.NotPanics(t, func() { s.delete("never-set") })
	require.NotPanics(t, func() { s.delete("") })
}

func TestSessionStreamsConcurrent(t *testing.T) {
	s := newSessionStreams()
	const n = 50
	var wg sync.WaitGroup
	for i := range n {
		id := strconv.Itoa(i) // precompute per iteration so each goroutine uses a distinct id
		wg.Go(func() {
			s.set(id, testStreamingClient(), nil)
			s.get(id)
			s.delete(id)
		})
	}
	wg.Wait()
	// All entries deleted; nothing left behind.
	assert.Equal(t, 0, s.count())
	for i := 0; i < n; i++ {
		assert.Nil(t, s.get(strconv.Itoa(i)))
	}
}
