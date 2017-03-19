// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package thriftudp

import (
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var localListenAddr = &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)}

func TestNewTUDPClientTransport(t *testing.T) {
	_, err := NewTUDPClientTransport("fakeAddressAndPort", "")
	require.NotNil(t, err)

	_, err = NewTUDPClientTransport("localhost:9090", "fakeaddressandport")
	require.NotNil(t, err)

	withLocalServer(t, func(addr string) {
		trans, err := NewTUDPClientTransport(addr, "")
		require.Nil(t, err)
		require.True(t, trans.IsOpen())
		require.NotNil(t, trans.Addr())

		//Check address
		assert.True(t, strings.HasPrefix(trans.Addr().String(), "127.0.0.1:"), "address check")
		require.Equal(t, "udp", trans.Addr().Network())

		err = trans.Open()
		require.Nil(t, err)

		err = trans.Close()
		require.Nil(t, err)
		require.False(t, trans.IsOpen())
	})
}

func TestNewTUDPServerTransport(t *testing.T) {
	_, err := NewTUDPServerTransport("fakeAddressAndPort")
	require.NotNil(t, err)

	trans, err := NewTUDPServerTransport(localListenAddr.String())
	require.Nil(t, err)
	require.True(t, trans.IsOpen())
	require.Equal(t, ^uint64(0), trans.RemainingBytes())

	//Ensure a second server can't be created on the same address
	trans2, err := NewTUDPServerTransport(trans.Addr().String())
	if trans2 != nil {
		//close the second server if one got created
		trans2.Close()
	}
	require.NotNil(t, err)

	err = trans.Close()
	require.Nil(t, err)
	require.False(t, trans.IsOpen())
}

func TestTUDPServerTransportIsOpen(t *testing.T) {
	_, err := NewTUDPServerTransport("fakeAddressAndPort")
	require.NotNil(t, err)

	trans, err := NewTUDPServerTransport(localListenAddr.String())
	require.Nil(t, err)
	require.True(t, trans.IsOpen())
	require.Equal(t, ^uint64(0), trans.RemainingBytes())

	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		time.Sleep(2 * time.Millisecond)
		err = trans.Close()
		require.Nil(t, err)
		wg.Done()
	}()

	go func() {
		for i := 0; i < 4; i++ {
			time.Sleep(1 * time.Millisecond)
			trans.IsOpen()
		}
		wg.Done()
	}()

	wg.Wait()
	require.False(t, trans.IsOpen())
}

func TestWriteRead(t *testing.T) {
	server, err := NewTUDPServerTransport(localListenAddr.String())
	require.Nil(t, err)
	defer server.Close()

	client, err := NewTUDPClientTransport(server.Addr().String(), "")
	require.Nil(t, err)
	defer client.Close()

	n, err := client.Write([]byte("test"))
	require.Nil(t, err)
	require.Equal(t, 4, n)
	n, err = client.Write([]byte("string"))
	require.Nil(t, err)
	require.Equal(t, 6, n)
	err = client.Flush()
	require.Nil(t, err)

	expected := []byte("teststring")
	readBuf := make([]byte, 20)
	n, err = server.Read(readBuf)
	require.Nil(t, err)
	require.Equal(t, len(expected), n)
	require.Equal(t, expected, readBuf[0:n])
}

func TestDoubleCloseError(t *testing.T) {
	trans, err := NewTUDPServerTransport(localListenAddr.String())
	require.Nil(t, err)
	require.True(t, trans.IsOpen())

	//Close connection object directly
	conn := trans.Conn()
	require.NotNil(t, conn)
	conn.Close()

	err = trans.Close()
	require.Error(t, err, "must return error when underlying connection is closed")

	assert.Equal(t, errConnAlreadyClosed, trans.Close(), "second Close() returns an error")
}

func TestConnClosedReadWrite(t *testing.T) {
	trans, err := NewTUDPServerTransport(localListenAddr.String())
	require.Nil(t, err)
	require.True(t, trans.IsOpen())
	require.NoError(t, trans.Close())
	require.False(t, trans.IsOpen())

	_, err = trans.Read(make([]byte, 1))
	require.NotNil(t, err)
	_, err = trans.Write([]byte("test"))
	require.NotNil(t, err)
}

func TestHugeWrite(t *testing.T) {
	withLocalServer(t, func(addr string) {
		trans, err := NewTUDPClientTransport(addr, "")
		require.Nil(t, err)

		hugeMessage := make([]byte, 40000)
		_, err = trans.Write(hugeMessage)
		require.Nil(t, err)

		//expect buffer to exceed max
		_, err = trans.Write(hugeMessage)
		require.NotNil(t, err)
	})
}

func TestFlushErrors(t *testing.T) {
	withLocalServer(t, func(addr string) {
		trans, err := NewTUDPClientTransport(addr, "")
		require.Nil(t, err)

		//flushing closed transport
		trans.Close()
		err = trans.Flush()
		require.NotNil(t, err)

		//error when trying to write in flush
		trans, err = NewTUDPClientTransport(addr, "")
		require.Nil(t, err)
		trans.conn.Close()

		trans.Write([]byte{1, 2, 3, 4})
		err = trans.Flush()
		require.Error(t, trans.Flush(), "Flush with data should fail")
	})
}

func TestResetInFlush(t *testing.T) {
	conn, err := net.ListenUDP(localListenAddr.Network(), localListenAddr)
	require.NoError(t, err, "ListenUDP failed")

	trans, err := NewTUDPClientTransport(conn.LocalAddr().String(), "")
	require.Nil(t, err)

	trans.Write([]byte("some nonsense"))
	trans.conn.Close() // close the transport's connection via back door

	err = trans.Flush()
	require.NotNil(t, err, "should fail to write to closed connection")
	assert.Equal(t, 0, trans.writeBuf.Len(), "should reset the buffer")
}

func withLocalServer(t *testing.T, f func(addr string)) {
	conn, err := net.ListenUDP(localListenAddr.Network(), localListenAddr)
	require.NoError(t, err, "ListenUDP failed")

	f(conn.LocalAddr().String())
	require.NoError(t, conn.Close(), "Close failed")
}

func TestCreateClient(t *testing.T) {
	_, err := createClient(nil, nil)
	assert.EqualError(t, err, "dial udp: missing address")
}
