// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/extension"
)

func Test_NewFactory(t *testing.T) {
	factory := NewFactory()
	assert.Equal(t, componentType, factory.Type())
	assert.Equal(t, factory.CreateDefaultConfig(), createDefaultConfig())
}

func Test_CreateExtension(t *testing.T) {
	set := extension.Settings{
		ID: ID,
	}
	cfg := createDefaultConfig()
	ext, err := createExtension(context.Background(), set, cfg)

	require.NoError(t, err)
	assert.NotNil(t, ext)
}
