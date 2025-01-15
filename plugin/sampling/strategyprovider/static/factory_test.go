// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package static

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/sampling/samplingstrategy"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/plugin"
)

var (
	_ samplingstrategy.Factory = new(Factory)
	_ plugin.Configurable      = new(Factory)
)

func TestFactory(t *testing.T) {
	f := NewFactory()
	v, command := config.Viperize(f.AddFlags)
	command.ParseFlags([]string{"--sampling.strategies-file=fixtures/strategies.json"})
	f.InitFromViper(v, zap.NewNop())

	require.NoError(t, f.Initialize(metrics.NullFactory, nil, zap.NewNop()))
	_, _, err := f.CreateStrategyProvider()
	require.NoError(t, err)
	require.NoError(t, f.Close())
}
