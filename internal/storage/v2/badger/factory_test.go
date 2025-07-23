// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package badger

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/badger"
)

func TestNewFac(t *testing.T) {
	f, err := NewFactory(*badger.DefaultConfig(), metrics.NullFactory, zaptest.NewLogger(t))
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
	cfg := badger.Config{}
	_, err := NewFactory(cfg, metrics.NullFactory, zaptest.NewLogger(t))
	require.ErrorContains(t, err, "Error Creating Dir: \"\" err: mkdir : no such file or directory")

	tmp := os.TempDir()
	defer os.Remove(tmp)
	cfg = badger.Config{
		Directories: badger.Directories{
			Keys:   tmp,
			Values: tmp,
		},
		Ephemeral:             false,
		MaintenanceInterval:   5,
		MetricsUpdateInterval: 10,
	}
	factory, err := NewFactory(cfg, metrics.NullFactory, zaptest.NewLogger(t))
	require.NoError(t, err)
	defer factory.Close()
}
