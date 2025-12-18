// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/telemetry"
)

func TestNewFactory(t *testing.T) {
	f, err := NewFactory(Configuration{MaxTraces: 10}, telemetry.NoopSettings())
	require.NoError(t, err)
	_, err = f.CreateTraceWriter()
	require.NoError(t, err)
	_, err = f.CreateTraceReader()
	require.NoError(t, err)
	_, err = f.CreateDependencyReader()
	require.NoError(t, err)
	_, err = f.CreateSamplingStore(5)
	require.NoError(t, err)
	_, err = f.CreateLock()
	require.NoError(t, err)
	require.NoError(t, f.Purge(context.Background()))
}

func TestNewFactoryErr(t *testing.T) {
	f, err := NewFactory(Configuration{}, telemetry.NoopSettings())
	require.ErrorContains(t, err, "max traces must be greater than zero")
	assert.Nil(t, f)
}
