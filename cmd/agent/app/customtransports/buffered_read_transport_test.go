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
