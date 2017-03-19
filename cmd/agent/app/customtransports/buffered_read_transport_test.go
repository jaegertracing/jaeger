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
