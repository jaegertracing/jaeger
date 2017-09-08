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
