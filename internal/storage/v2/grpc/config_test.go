// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/xconfmap"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	require.NotEmpty(t, cfg.Timeout)
	require.Zero(t, cfg.MaxRecvMsgSizeMiB, "default should use gRPC's built-in 4 MiB limit")
}

func TestConfigUnmarshalNormalizesBalancerNames(t *testing.T) {
	cfg := DefaultConfig()
	conf := confmap.NewFromStringMap(map[string]any{
		"endpoint":      "localhost:17271",
		"balancer_name": "ROUND_ROBIN",
		"writer": map[string]any{
			"endpoint":      "localhost:17272",
			"balancer_name": "PICK_FIRST",
		},
	})

	require.NoError(t, conf.Unmarshal(&cfg))
	require.Equal(t, "round_robin", cfg.BalancerName)
	require.Equal(t, "pick_first", cfg.Writer.BalancerName)
	require.NotEmpty(t, cfg.Timeout)
	require.NoError(t, xconfmap.Validate(&cfg))
}

func TestConfigUnmarshalError(t *testing.T) {
	conf := confmap.NewFromStringMap(map[string]any{
		"max_recv_msg_size_mib": "invalid",
	})

	require.Error(t, conf.Unmarshal(&Config{}))
}
