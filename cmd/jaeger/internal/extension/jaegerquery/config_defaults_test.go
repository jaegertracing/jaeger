// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	require.Equal(t, ":16686", cfg.HTTP.Endpoint)
	require.Equal(t, ":16685", cfg.GRPC.NetAddr.Endpoint)
	require.EqualValues(t, "tcp", cfg.GRPC.NetAddr.Transport)
}
