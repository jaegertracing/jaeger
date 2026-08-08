// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegercli

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
)

func TestComponents(t *testing.T) {
	factories, err := Components()
	require.NoError(t, err)

	// The IDs are pinned explicitly so that a component added to or removed
	// from the standard binary fails this test. The otlp_grpc, otlphttp, and
	// spanmetrics entries are deprecated aliases registered alongside the
	// canonical types.
	assert.Equal(t, []string{
		"basicauth", "expvar", "healthcheckv2", "jaeger_query",
		"jaeger_storage", "pprof", "remote_sampling", "remote_storage",
		"sigv4auth", "storage_cleaner", "zpages",
	}, keys(factories.Extensions))
	assert.Equal(t, []string{
		"jaeger", "kafka", "nop", "otlp", "zipkin",
	}, keys(factories.Receivers))
	assert.Equal(t, []string{
		"debug", "jaeger_storage_exporter", "kafka", "nop", "otlp",
		"otlp_grpc", "otlp_http", "otlphttp", "prometheus",
	}, keys(factories.Exporters))
	assert.Equal(t, []string{
		"adaptive_sampling", "attributes", "batch", "filter",
		"memory_limiter", "tail_sampling",
	}, keys(factories.Processors))
	assert.Equal(t, []string{
		"forward", "span_metrics", "spanmetrics",
	}, keys(factories.Connectors))
	assert.NotNil(t, factories.Telemetry)
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
