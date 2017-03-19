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
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTBufferedReadTransport tests the TBufferedReadTransport
func TestTBufferedReadTransport(t *testing.T) {
	buffer := bytes.NewBuffer([]byte("testString"))
	trans, err := NewTBufferedReadTransport(buffer)
	require.NotNil(t, trans)
	require.Nil(t, err)
	require.Equal(t, uint64(10), trans.RemainingBytes())

	firstRead := make([]byte, 4)
	n, err := trans.Read(firstRead)
	require.Nil(t, err)
	require.Equal(t, 4, n)
	require.Equal(t, []byte("test"), firstRead)
	require.Equal(t, uint64(6), trans.RemainingBytes())

	secondRead := make([]byte, 7)
	n, err = trans.Read(secondRead)
	require.Equal(t, 6, n)
	require.Equal(t, []byte("String"), secondRead[0:6])
	require.Equal(t, uint64(0), trans.RemainingBytes())
}

// TestTBufferedReadTransportEmptyFunctions tests the empty functions in TBufferedReadTransport
func TestTBufferedReadTransportEmptyFunctions(t *testing.T) {
	byteArr := make([]byte, 1)
	trans, err := NewTBufferedReadTransport(bytes.NewBuffer(byteArr))
	require.NotNil(t, trans)
	require.Nil(t, err)

	err = trans.Open()
	require.Nil(t, err)

	err = trans.Close()
	require.Nil(t, err)

	err = trans.Flush()
	require.Nil(t, err)

	n, err := trans.Write(byteArr)
	require.Equal(t, 1, n)
	require.Nil(t, err)

	isOpen := trans.IsOpen()
	require.True(t, isOpen)
}
