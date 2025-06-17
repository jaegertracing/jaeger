// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package cassandra

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra"
)

func TestNewFactoryErr(t *testing.T) {
	opts := cassandra.Options{}
	f, err := NewFactory(opts, metrics.NullFactory, zap.NewNop())
	require.ErrorContains(t, err, "Servers: non zero value required")
	assert.Nil(t, f)
}

func TestNewFactory(t *testing.T) {
	v2Factory, err := newFactory(func() (v1Factory *cassandra.Factory, err error) {
		return &cassandra.Factory{}, nil
	})
	require.NoError(t, err)
	assert.NotNil(t, v2Factory)
}
