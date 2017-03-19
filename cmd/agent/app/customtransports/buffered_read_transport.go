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

package customtransport

import (
	"bytes"

	"github.com/apache/thrift/lib/go/thrift"
)

// TBufferedReadTransport is a thrift.TTransport that reads from a buffer
type TBufferedReadTransport struct {
	readBuf *bytes.Buffer
}

// NewTBufferedReadTransport creates a buffer backed TTransport
func NewTBufferedReadTransport(readBuf *bytes.Buffer) (*TBufferedReadTransport, error) {
	return &TBufferedReadTransport{readBuf: readBuf}, nil
}

// IsOpen does nothing as transport is not maintaining the connection
// Required to maintain thrift.TTransport interface
func (p *TBufferedReadTransport) IsOpen() bool {
	return true
}

// Open does nothing as transport is not maintaining the connection
// Required to maintain thrift.TTransport interface
func (p *TBufferedReadTransport) Open() error {
	return nil
}

// Close does nothing as transport is not maintaining the connection
// Required to maintain thrift.TTransport interface
func (p *TBufferedReadTransport) Close() error {
	return nil
}

// Read reads bytes from the local buffer and puts them in the specified buf
func (p *TBufferedReadTransport) Read(buf []byte) (int, error) {
	in, err := p.readBuf.Read(buf)
	return in, thrift.NewTTransportExceptionFromError(err)
}

// RemainingBytes returns the number of bytes left to be read from the readBuf
func (p *TBufferedReadTransport) RemainingBytes() uint64 {
	return uint64(p.readBuf.Len())
}

// Write writes bytes into the read buffer
// Required to maintain thrift.TTransport interface
func (p *TBufferedReadTransport) Write(buf []byte) (int, error) {
	p.readBuf = bytes.NewBuffer(buf)
	return len(buf), nil
}

// Flush does nothing as udp server does not write responses back
// Required to maintain thrift.TTransport interface
func (p *TBufferedReadTransport) Flush() error {
	return nil
}
