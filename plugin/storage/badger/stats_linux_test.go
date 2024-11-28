// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package badger

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/telemetry"
)

func TestDiskStatisticsUpdate(t *testing.T) {
	telset := telemetry.NoopSettings()
	mFactory := metricstest.NewFactory(0)
	telset.Metrics = mFactory
	f := NewFactory(telset)
	cfg := DefaultConfig()
	v, command := config.Viperize(cfg.AddFlags)
	command.ParseFlags([]string{
		"--badger.ephemeral=true",
		"--badger.consistency=false",
	})
	f.InitFromViper(v, zap.NewNop())
	err := f.Initialize()
	require.NoError(t, err)
	defer f.Close()

	err = f.diskStatisticsUpdate()
	require.NoError(t, err)
	_, gs := mFactory.Snapshot()
	assert.Positive(t, gs[keyLogSpaceAvailableName])
	assert.Positive(t, gs[valueLogSpaceAvailableName])
}
