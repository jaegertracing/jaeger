// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

func TestNewFactory(t *testing.T) {
	f, err := NewFactory(Configuration{MaxTraces: 10}, telemetry.NoopSettings(), tenancy.Options{})
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
	f, err := NewFactory(Configuration{}, telemetry.NoopSettings(), tenancy.Options{})
	require.ErrorContains(t, err, "max traces must be greater than zero")
	assert.Nil(t, f)
}

func TestNewFactoryRejectsUnrestrictedTenancy(t *testing.T) {
	f, err := NewFactory(Configuration{MaxTraces: 10}, telemetry.NoopSettings(), tenancy.Options{Enabled: true})
	require.ErrorContains(t, err, "multi-tenancy is enabled with an empty tenant list")
	require.Nil(t, f)
}

func TestFactoryApplyTenancyAfterConstruction(t *testing.T) {
	f, err := NewFactory(Configuration{MaxTraces: 10}, telemetry.NoopSettings(), tenancy.Options{})
	require.NoError(t, err)
	f.ApplyTenancy(tenancy.Options{Enabled: true, Tenants: []string{"allowed"}})

	_, err = f.store.GetServices(tenancy.WithTenant(context.Background(), "outside"))
	require.ErrorIs(t, err, errUnknownTenant)
	_, err = f.store.GetServices(tenancy.WithTenant(context.Background(), "allowed"))
	require.NoError(t, err)
}
