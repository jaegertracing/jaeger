// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package testutils

import (
	"testing"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/stretchr/testify/require"
)

func TestNewZipkinThriftUDPClient(t *testing.T) {
	_, _, err := NewZipkinThriftUDPClient("256.2.3:0")
	require.Error(t, err)

	_, cl, err := NewZipkinThriftUDPClient("127.0.0.1:12345")
	require.NoError(t, err)
	cl.Close()
}

func TestNewJaegerThriftUDPClient(t *testing.T) {
	compactFactory := thrift.NewTCompactProtocolFactoryConf(&thrift.TConfiguration{})

	_, _, err := NewJaegerThriftUDPClient("256.2.3:0", compactFactory)
	require.Error(t, err)

	_, cl, err := NewJaegerThriftUDPClient("127.0.0.1:12345", compactFactory)
	require.NoError(t, err)
	cl.Close()
}
