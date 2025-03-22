// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package badger

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/config"
	"github.com/jaegertracing/jaeger/internal/metricstest"
)

func TestDiskStatisticsUpdate(t *testing.T) {
	f := NewFactory()
	cfg := DefaultConfig()
	v, command := config.Viperize(cfg.AddFlags)
	command.ParseFlags([]string{
		"--badger.ephemeral=true",
		"--badger.consistency=false",
	})
	f.InitFromViper(v, zap.NewNop())
	mFactory := metricstest.NewFactory(0)
	err := f.Initialize(mFactory, zap.NewNop())
	require.NoError(t, err)
	defer f.Close()

	err = f.diskStatisticsUpdate()
	require.NoError(t, err)
	_, gs := mFactory.Snapshot()
	assert.Positive(t, gs[keyLogSpaceAvailableName])
	assert.Positive(t, gs[valueLogSpaceAvailableName])
}
