// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package servers

import (
	"bytes"
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/agent/app/servers/thriftudp"
)

func TestUDPServerSendReceive(t *testing.T) {
	transport, err := thriftudp.NewTUDPServerTransport("127.0.0.1:0")
	require.NoError(t, err)

	var packerRecieved atomic.Bool
	packerRecieved.Store(false)
	handler := func(buf *bytes.Buffer, release func(*bytes.Buffer)) {
		defer release(buf)
		assert.Positive(t, buf.Len())
		assert.Equal(t, "span1", buf.String())
		packerRecieved.Store(true)
	}

	const maxPacketSize = 65000
	const maxQueueSize = 100
	server, err := NewUDPServer(transport, handler, maxPacketSize)
	require.NoError(t, err)
	go server.Serve()
	defer server.Stop()

	client, err := thriftudp.NewTUDPClientTransport(transport.Addr().String(), "")
	require.NoError(t, err)
	defer client.Close()

	// keep sending packets until the server receives one
	for range 1000 {
		n, err := client.Write([]byte("span1"))
		require.NoError(t, err)
		require.Equal(t, 5, n)
		require.NoError(t, client.Flush(context.Background()))

		if packerRecieved.Load() {
			return // exit test on successful receipt
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("server did not receive packets")
}

func TestUDPServerStopBeforeServe(t *testing.T) {
	transport, err := thriftudp.NewTUDPServerTransport("127.0.0.1:0")
	require.NoError(t, err)
	server, err := NewUDPServer(transport, nil, 65000)
	require.NoError(t, err)
	server.Stop()
	server.Serve()
	server.Stop() // stop should be idempotent
}
