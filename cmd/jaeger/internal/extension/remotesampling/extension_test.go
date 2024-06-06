// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package remotesampling

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
)

func TestNewExtension(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.File.Path = filepath.Join("..", "..", "..", "sampling-strategies.json")
	e := newExtension(cfg, componenttest.NewNopTelemetrySettings())

	assert.NotNil(t, e)
}

func TestStartAndShutdownLocalFile(t *testing.T) {
	cfg := createDefaultConfig().(*Config)
	cfg.File.Path = filepath.Join("..", "..", "..", "sampling-strategies.json")

	e := newExtension(cfg, componenttest.NewNopTelemetrySettings())
	require.NotNil(t, e)
	require.NoError(t, e.Start(context.Background(), componenttest.NewNopHost()))

	require.NoError(t, e.Shutdown(context.Background()))
}
