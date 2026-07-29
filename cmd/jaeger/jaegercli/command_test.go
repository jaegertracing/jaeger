// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegercli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal"
)

func TestNewCommand(t *testing.T) {
	factories, err := internal.Components()
	require.NoError(t, err)

	cmd := NewCommand(factories)

	require.NotNil(t, cmd)
	assert.Equal(t, "jaeger", cmd.Use)
}
