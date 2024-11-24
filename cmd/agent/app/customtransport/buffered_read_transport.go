// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package customtransport

import (
	"bytes"
	"context"

	"github.com/apache/thrift/lib/go/thrift"
)

// TBufferedReadTransport is a thrift.TTransport that reads from a buffer
type TBufferedReadTransport struct {
	readBuf *bytes.Buffer
}

var _ thrift.TTransport = (*TBufferedReadTransport)(nil)

// NewTBufferedReadTransport creates a buffer backed TTransport
func NewTBufferedReadTransport(readBuf *bytes.Buffer) (*TBufferedReadTransport, error) {
	return &TBufferedReadTransport{readBuf: readBuf}, nil
}

// IsOpen does nothing as transport is not maintaining the connection
// Required to maintain thrift.TTransport interface
func (*TBufferedReadTransport) IsOpen() bool {
	return true
}

// Open does nothing as transport is not maintaining the connection
// Required to maintain thrift.TTransport interface
func (*TBufferedReadTransport) Open() error {
	return nil
}

// Close does nothing as transport is not maintaining the connection
// Required to maintain thrift.TTransport interface
func (*TBufferedReadTransport) Close() error {
	return nil
}

// Read reads bytes from the local buffer and puts them in the specified buf
func (p *TBufferedReadTransport) Read(buf []byte) (int, error) {
	in, err := p.readBuf.Read(buf)
	return in, thrift.NewTTransportExceptionFromError(err)
}

// RemainingBytes returns the number of bytes left to be read from the readBuf
func (p *TBufferedReadTransport) RemainingBytes() uint64 {
	//nolint: gosec // G115
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
func (*TBufferedReadTransport) Flush(_ context.Context) error {
	return nil
}
