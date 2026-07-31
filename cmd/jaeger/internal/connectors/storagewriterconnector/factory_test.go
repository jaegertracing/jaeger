// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storagewriterconnector

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/connector/connectortest"
	"go.opentelemetry.io/collector/consumer/consumertest"
)

func TestNewFactory_CreatesTracesToTraces(t *testing.T) {
	f := NewFactory()
	assert.Equal(t, componentType, f.Type())

	cfg := f.CreateDefaultConfig()
	require.NoError(t, componenttest.CheckConfigStruct(cfg))

	// The default config is missing the required trace_storage, so it is invalid.
	require.Error(t, cfg.(*Config).Validate())

	cfg.(*Config).TraceStorage = "somestore"
	require.NoError(t, cfg.(*Config).Validate())

	conn, err := f.CreateTracesToTraces(
		context.Background(),
		connectortest.NewNopSettings(componentType),
		cfg,
		consumertest.NewNop(),
	)
	require.NoError(t, err)
	assert.NotNil(t, conn)
	assert.False(t, conn.Capabilities().MutatesData)
}
