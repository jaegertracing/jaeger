// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
)

func Test_getUniqueOperationNames(t *testing.T) {
	assert.Equal(t, []string(nil), getUniqueOperationNames(nil))

	operations := []spanstore.Operation{
		{Name: "operation1", SpanKind: "server"},
		{Name: "operation1", SpanKind: "client"},
		{Name: "operation2"},
	}

	expNames := []string{"operation1", "operation2"}
	names := getUniqueOperationNames(operations)
	assert.Len(t, names, 2)
	assert.ElementsMatch(t, expNames, names)
}

func Test_getPort(t *testing.T) {
	cases := []struct {
		address       testAddr
		expected      int
		expectedError string
	}{
		{
			address: testAddr{
				Address: "localhost:0",
			},
			expected: 0,
		},
		{
			address: testAddr{
				Address: "localhost",
			},
			expectedError: "address localhost: missing port in address",
		},
		{
			address: testAddr{
				Address: "localhost:port",
			},
			expectedError: "strconv.Atoi: parsing \"port\": invalid syntax",
		},
	}
	for _, c := range cases {
		port, err := getPortForAddr(&c.address)
		if c.expectedError != "" {
			require.EqualError(t, err, c.expectedError)
		} else {
			assert.Equal(t, c.expected, port)
		}
	}
}

type testAddr struct {
	Address string
}

// Implement net.Addr methods
func (*testAddr) Network() string {
	return "tcp"
}

func (tA *testAddr) String() string {
	return tA.Address
}
