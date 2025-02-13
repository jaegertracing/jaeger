// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/testutils"
)

func TestNewClientWithDefaults(t *testing.T) {
	cfg := DefaultConfiguration()
	logger := zap.NewNop()
	client, err := cfg.NewClient(logger)
	require.NoError(t, err)
	assert.NotEmpty(t, client)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
