// Copyright (c) 2021 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package disabled

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/pkg/telemetry"
)

var _ storage.MetricStoreFactory = new(Factory)

func TestPrometheusFactory(t *testing.T) {
	f := NewFactory()
	require.NoError(t, f.Initialize(telemetry.NoopSettings()))

	err := f.Initialize(telemetry.NoopSettings())
	require.NoError(t, err)

	f.AddFlags(nil)
	f.InitFromViper(nil, zap.NewNop())

	reader, err := f.CreateMetricsReader()
	require.NoError(t, err)
	assert.NotNil(t, reader)
}
