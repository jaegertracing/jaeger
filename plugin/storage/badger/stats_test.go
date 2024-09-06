// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

//go:build !linux
// +build !linux

package badger

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
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
	err := f.Initialize(metrics.NullFactory, zap.NewNop())
	require.NoError(t, err)
	defer f.Close()

	// We're not expecting any value in !linux, just no error
	err = f.diskStatisticsUpdate()
	require.NoError(t, err)
}
