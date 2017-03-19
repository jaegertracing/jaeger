package testutils

import (
	"testing"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewZipkinThriftUDPClient(t *testing.T) {
	_, _, err := NewZipkinThriftUDPClient("1.2.3:0")
	assert.Error(t, err)

	_, cl, err := NewZipkinThriftUDPClient("127.0.0.1:12345")
	require.NoError(t, err)
	cl.Close()
}

func TestNewJaegerThriftUDPClient(t *testing.T) {
	compactFactory := thrift.NewTCompactProtocolFactory()

	_, _, err := NewJaegerThriftUDPClient("1.2.3:0", compactFactory)
	assert.Error(t, err)

	_, cl, err := NewJaegerThriftUDPClient("127.0.0.1:12345", compactFactory)
	require.NoError(t, err)
	cl.Close()
}
