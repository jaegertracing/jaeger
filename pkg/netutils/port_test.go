// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package netutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/testutils"
)

type testAddr struct {
	Address string
}

func (*testAddr) Network() string {
	return "tcp"
}

func (tA *testAddr) String() string {
	return tA.Address
}

func TestGetPort(t *testing.T) {
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
		port, err := GetPort(&c.address)
		if c.expectedError != "" {
			require.EqualError(t, err, c.expectedError)
		} else {
			assert.Equal(t, c.expected, port)
		}
	}
}

func TestFixLocalhost(t *testing.T) {
	endpoints := []string{"collector:1111", ":2222"}
	fixed := FixLocalhost(endpoints)
	assert.Equal(t, []string{"collector:1111", "localhost:2222"}, fixed)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
