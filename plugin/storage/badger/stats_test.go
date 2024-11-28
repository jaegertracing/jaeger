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
	"github.com/jaegertracing/jaeger/pkg/telemetry"
)

func TestDiskStatisticsUpdate(t *testing.T) {
	f := NewFactory(telemetry.NoopSettings())
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

	// We're not expecting any value in !linux, just no error
	err = f.diskStatisticsUpdate()
	require.NoError(t, err)
}
