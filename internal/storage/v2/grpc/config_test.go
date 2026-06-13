// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	require.NotEmpty(t, cfg.Timeout)
	require.Zero(t, cfg.MaxRecvMsgSizeMiB, "default should use gRPC's built-in 4 MiB limit")
}
