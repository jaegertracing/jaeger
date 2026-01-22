// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package badger

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/internal/storage/v1/badger"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

func TestNewFac(t *testing.T) {
	telset := telemetry.NoopSettings()
	telset.Logger = zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
	f, err := NewFactory(*badger.DefaultConfig(), telset)
	require.NoError(t, err)

	_, err = f.CreateTraceReader()
	require.NoError(t, err)

	_, err = f.CreateTraceWriter()
	require.NoError(t, err)

	_, err = f.CreateDependencyReader()
	require.NoError(t, err)

	_, err = f.CreateSamplingStore(5)
	require.NoError(t, err)

	lock, err := f.CreateLock()
	require.NoError(t, err)
	assert.NotNil(t, lock)

	err = f.Purge(context.Background())
	require.NoError(t, err)

	err = f.Close()
	require.NoError(t, err)
}

func TestBadgerStorageFactoryWithConfig(t *testing.T) {
	t.Parallel()
	cfg := badger.Config{}
	_, err := NewFactory(cfg, telemetry.NoopSettings())
	require.ErrorContains(t, err, "Error Creating Dir: \"\" err: mkdir : no such file or directory")

	cfg = badger.Config{
		Ephemeral:             true,
		MaintenanceInterval:   5,
		MetricsUpdateInterval: 10,
	}
	factory, err := NewFactory(cfg, telemetry.NoopSettings())
	require.NoError(t, err)
	factory.Close()
}
