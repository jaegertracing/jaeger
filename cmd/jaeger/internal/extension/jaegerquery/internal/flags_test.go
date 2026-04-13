// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultQueryOptions(t *testing.T) {
	qo := DefaultQueryOptions()
	require.Equal(t, ":16686", qo.HTTP.NetAddr.Endpoint)
	require.Equal(t, ":16685", qo.GRPC.NetAddr.Endpoint)
	require.EqualValues(t, "tcp", qo.GRPC.NetAddr.Transport)
	require.False(t, qo.AI.HasValue())
	aiCfg := qo.AI.GetOrInsertDefault()
	require.NotNil(t, aiCfg)
	require.Equal(t, "ws://localhost:16688", aiCfg.AgentURL)
	require.Equal(t, 50*time.Millisecond, aiCfg.WaitForTurnTimeout)
	require.Equal(t, int64(1<<20), aiCfg.MaxRequestBodySize)
	require.NoError(t, aiCfg.Validate())
}

func TestAIConfigValidateRejectsNegativeBodySize(t *testing.T) {
	cfg := AIConfig{MaxRequestBodySize: -1}
	require.Error(t, cfg.Validate())
}

func TestAIConfigValidateAcceptsZeroBodySize(t *testing.T) {
	cfg := AIConfig{MaxRequestBodySize: 0}
	require.NoError(t, cfg.Validate())
}

func TestAIConfigValidateRejectsNegativeTimeout(t *testing.T) {
	cfg := AIConfig{WaitForTurnTimeout: -1}
	require.Error(t, cfg.Validate())
}
