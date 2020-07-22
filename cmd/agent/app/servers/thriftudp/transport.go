// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package thriftudp

import (
	"bytes"
	"context"
	"errors"
	"net"
	"sync/atomic"

	"github.com/apache/thrift/lib/go/thrift"
)

//MaxLength of UDP packet
const (
	MaxLength = 65000
)

var errConnAlreadyClosed = errors.New("connection already closed")

// TUDPTransport does UDP as a thrift.TTransport
type TUDPTransport struct {
	conn     *net.UDPConn
	addr     net.Addr
	writeBuf bytes.Buffer
	closed   uint32 // atomic flag
}

var _ thrift.TTransport = (*TUDPTransport)(nil)

// NewTUDPClientTransport creates a net.UDPConn-backed TTransport for Thrift clients
// All writes are buffered and flushed in one UDP packet. If locHostPort is not "", it
// will be used as the local address for the connection
// Example:
// 	trans, err := thriftudp.NewTUDPClientTransport("192.168.1.1:9090", "")
func NewTUDPClientTransport(destHostPort string, locHostPort string) (*TUDPTransport, error) {
	destAddr, err := net.ResolveUDPAddr("udp", destHostPort)
	if err != nil {
		return nil, thrift.NewTTransportException(thrift.NOT_OPEN, err.Error())
	}

	var locAddr *net.UDPAddr
	if locHostPort != "" {
		locAddr, err = net.ResolveUDPAddr("udp", locHostPort)
		if err != nil {
			return nil, thrift.NewTTransportException(thrift.NOT_OPEN, err.Error())
		}
	}

	return createClient(destAddr, locAddr)
}

func createClient(destAddr, locAddr *net.UDPAddr) (*TUDPTransport, error) {
	conn, err := net.DialUDP(destAddr.Network(), locAddr, destAddr)
	if err != nil {
		return nil, thrift.NewTTransportException(thrift.NOT_OPEN, err.Error())
	}
	return &TUDPTransport{addr: destAddr, conn: conn}, nil
}

// NewTUDPServerTransport creates a net.UDPConn-backed TTransport for Thrift servers
// It will listen for incoming udp packets on the specified host/port
// Example:
// 	trans, err := thriftudp.NewTUDPClientTransport("localhost:9001")
func NewTUDPServerTransport(hostPort string) (*TUDPTransport, error) {
	addr, err := net.ResolveUDPAddr("udp", hostPort)
	if err != nil {
		return nil, thrift.NewTTransportException(thrift.NOT_OPEN, err.Error())
	}
	conn, err := net.ListenUDP(addr.Network(), addr)
	if err != nil {
		return nil, thrift.NewTTransportException(thrift.NOT_OPEN, err.Error())
	}

	return &TUDPTransport{addr: conn.LocalAddr(), conn: conn}, nil
}

// Open does nothing as connection is opened on creation
// Required to maintain thrift.TTransport interface
func (p *TUDPTransport) Open() error {
	return nil
}

// Conn retrieves the underlying net.UDPConn
func (p *TUDPTransport) Conn() *net.UDPConn {
	return p.conn
}

// IsOpen returns true if the connection is open
func (p *TUDPTransport) IsOpen() bool {
	return atomic.LoadUint32(&p.closed) == 0
}

// Close closes the connection
func (p *TUDPTransport) Close() error {
	if atomic.CompareAndSwapUint32(&p.closed, 0, 1) {
		return p.conn.Close()
	}
	return errConnAlreadyClosed
}

// Addr returns the address that the transport is listening on or writing to
func (p *TUDPTransport) Addr() net.Addr {
	return p.addr
}

// Read reads one UDP packet and puts it in the specified buf
func (p *TUDPTransport) Read(buf []byte) (int, error) {
	if !p.IsOpen() {
		return 0, thrift.NewTTransportException(thrift.NOT_OPEN, "Connection not open")
	}
	n, err := p.conn.Read(buf)
	return n, thrift.NewTTransportExceptionFromError(err)
}

// RemainingBytes returns the max number of bytes (same as Thrift's StreamTransport) as we
// do not know how many bytes we have left.
func (p *TUDPTransport) RemainingBytes() uint64 {
	const maxSize = ^uint64(0)
	return maxSize
}

// Write writes specified buf to the write buffer
func (p *TUDPTransport) Write(buf []byte) (int, error) {
	if !p.IsOpen() {
		return 0, thrift.NewTTransportException(thrift.NOT_OPEN, "Connection not open")
	}
	if len(p.writeBuf.Bytes())+len(buf) > MaxLength {
		return 0, thrift.NewTTransportException(thrift.INVALID_DATA, "Data does not fit within one UDP packet")
	}
	n, err := p.writeBuf.Write(buf)
	return n, thrift.NewTTransportExceptionFromError(err)
}

// Flush flushes the write buffer as one udp packet
func (p *TUDPTransport) Flush(_ context.Context) error {
	if !p.IsOpen() {
		return thrift.NewTTransportException(thrift.NOT_OPEN, "Connection not open")
	}

	_, err := p.conn.Write(p.writeBuf.Bytes())
	p.writeBuf.Reset() // always reset the buffer, even in case of an error
	return err
}

// SetSocketBufferSize sets udp buffer size
func (p *TUDPTransport) SetSocketBufferSize(bufferSize int) error {
	return setSocketBuffer(p.Conn(), bufferSize)
}
