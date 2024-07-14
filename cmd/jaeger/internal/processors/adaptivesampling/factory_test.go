// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adaptivesampling

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/processor/processortest"
)

func TestCreateDefaultConfig(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	require.NotNil(t, cfg, "failed to create default config")
	require.NoError(t, componenttest.CheckConfigStruct(cfg))
	require.NoError(t, cfg.Validate())
}

func TestCreateTracesProcessor(t *testing.T) {
	ctx := context.Background()
	cfg := createDefaultConfig().(*Config)

	nextConsumer := consumertest.NewNop()
	set := processortest.NewNopSettings()

	tracesProcessor, err := createTracesProcessor(ctx, set, cfg, nextConsumer)
	require.NoError(t, err)
	assert.NotNil(t, tracesProcessor)
}

func TestFactoryType(t *testing.T) {
	factory := NewFactory()
	assert.Equal(t, componentType, factory.Type())
}
