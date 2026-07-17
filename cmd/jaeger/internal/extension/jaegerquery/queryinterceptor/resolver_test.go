// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package queryinterceptor

import (
	"context"
	"testing"

	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/storage/storagetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/ptrace"

	pub "github.com/jaegertracing/jaeger/components/extension/jaegerquery/queryinterceptor"
)

// stubInterceptor is both a component.Component (an OTel extension) and a public
// Interceptor — the shape a real plugin must have.
type stubInterceptor struct{}

func (stubInterceptor) Start(context.Context, component.Host) error { return nil }
func (stubInterceptor) Shutdown(context.Context) error              { return nil }
func (stubInterceptor) OnQuery(_ context.Context, q pub.Query) (pub.Query, error) {
	return q, nil
}

func (stubInterceptor) OnResult(_ context.Context, t []ptrace.Traces) ([]ptrace.Traces, error) {
	return t, nil
}

// plainExtension is an extension that does NOT implement Interceptor.
type plainExtension struct{}

func (plainExtension) Start(context.Context, component.Host) error { return nil }
func (plainExtension) Shutdown(context.Context) error              { return nil }

func TestResolve(t *testing.T) {
	interceptorID := component.MustNewID("query_interceptor_example")
	plainID := component.MustNewID("expvar")

	host := storagetest.NewStorageHost().
		WithExtension(interceptorID, stubInterceptor{}).
		WithExtension(plainID, plainExtension{})

	t.Run("no ids", func(t *testing.T) {
		got, err := Resolve(host, nil)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("resolves in order", func(t *testing.T) {
		got, err := Resolve(host, []component.ID{interceptorID})
		require.NoError(t, err)
		require.Len(t, got, 1)
	})

	t.Run("missing extension", func(t *testing.T) {
		_, err := Resolve(host, []component.ID{component.MustNewID("nonexistent")})
		require.ErrorContains(t, err, "cannot find query interceptor extension")
	})

	t.Run("wrong type", func(t *testing.T) {
		_, err := Resolve(host, []component.ID{plainID})
		require.ErrorContains(t, err, "does not implement queryinterceptor.Interceptor")
	})
}
