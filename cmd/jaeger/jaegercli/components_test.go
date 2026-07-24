// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegercli

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal"
)

func TestComponents(t *testing.T) {
	factories, err := Components()
	require.NoError(t, err)

	assert.Contains(t, factories.Extensions, component.MustNewType("jaeger_storage"))
	assert.Contains(t, factories.Receivers, component.MustNewType("jaeger"))
	assert.NotNil(t, factories.Telemetry)

	// the public set must stay at parity with the set the standard binary uses
	expected, err := internal.Components()
	require.NoError(t, err)
	assert.Equal(t, keys(expected.Extensions), keys(factories.Extensions))
	assert.Equal(t, keys(expected.Receivers), keys(factories.Receivers))
	assert.Equal(t, keys(expected.Exporters), keys(factories.Exporters))
	assert.Equal(t, keys(expected.Processors), keys(factories.Processors))
	assert.Equal(t, keys(expected.Connectors), keys(factories.Connectors))
}

func TestComponentsReturnsIndependentSet(t *testing.T) {
	jaegerStorage := component.MustNewType("jaeger_storage")

	factories, err := Components()
	require.NoError(t, err)
	require.Contains(t, factories.Extensions, jaegerStorage)
	delete(factories.Extensions, jaegerStorage)

	next, err := Components()
	require.NoError(t, err)
	assert.Contains(t, next.Extensions, jaegerStorage)
}

func keys[F any](m map[component.Type]F) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k.String())
	}
	slices.Sort(out)
	return out
}
