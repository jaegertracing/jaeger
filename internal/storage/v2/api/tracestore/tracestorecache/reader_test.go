// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestorecache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	tracestoremocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

const testTTL = time.Minute

type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.now = c.now.Add(d)
}

func newTestReader(t *testing.T) (*Reader, *tracestoremocks.Reader, *fakeClock) {
	backend := tracestoremocks.NewReader(t)
	clock := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	r := NewReader(backend, testTTL, zap.NewNop())
	r.timeNow = clock.Now
	return r, backend, clock
}

func withTenant(tenant string) any {
	return mock.MatchedBy(func(ctx context.Context) bool {
		return tenancy.GetTenant(ctx) == tenant
	})
}

func TestGetServicesReusesCachedValuesUntilTTLExpires(t *testing.T) {
	r, backend, clock := newTestReader(t)
	backend.On("GetServices", mock.Anything).Return([]string{"a", "b"}, nil).Once()

	services, err := r.GetServices(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, services)

	clock.Advance(testTTL - time.Nanosecond)
	services, err = r.GetServices(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, services)

	backend.On("GetServices", mock.Anything).Return([]string{"a", "b", "c"}, nil).Once()
	clock.Advance(time.Nanosecond)
	services, err = r.GetServices(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, services)

	services, err = r.GetServices(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, services)
}

func TestGetServicesKeepsTenantsApart(t *testing.T) {
	r, backend, _ := newTestReader(t)
	backend.On("GetServices", withTenant("blue")).Return([]string{"blue-svc"}, nil).Once()
	backend.On("GetServices", withTenant("green")).Return([]string{"green-svc"}, nil).Once()
	blue := tenancy.WithTenant(t.Context(), "blue")
	green := tenancy.WithTenant(t.Context(), "green")

	for range 2 {
		services, err := r.GetServices(blue)
		require.NoError(t, err)
		assert.Equal(t, []string{"blue-svc"}, services)

		services, err = r.GetServices(green)
		require.NoError(t, err)
		assert.Equal(t, []string{"green-svc"}, services)
	}
}

func TestGetOperationsCachesEachQuerySeparately(t *testing.T) {
	r, backend, _ := newTestReader(t)
	queries := map[tracestore.OperationQueryParams][]tracestore.Operation{
		{ServiceName: "svc1"}:                     {{Name: "all-ops"}},
		{ServiceName: "svc1", SpanKind: "server"}: {{Name: "server-op", SpanKind: "server"}},
		{ServiceName: "svc2"}:                     {{Name: "other-op"}},
	}
	for query, operations := range queries {
		backend.On("GetOperations", withTenant(""), query).Return(operations, nil).Once()
	}
	backend.On("GetOperations", withTenant("blue"), tracestore.OperationQueryParams{ServiceName: "svc1"}).
		Return([]tracestore.Operation{{Name: "blue-op"}}, nil).Once()

	for range 2 {
		for query, want := range queries {
			operations, err := r.GetOperations(t.Context(), query)
			require.NoError(t, err)
			assert.Equal(t, want, operations)
		}
		operations, err := r.GetOperations(tenancy.WithTenant(t.Context(), "blue"), tracestore.OperationQueryParams{ServiceName: "svc1"})
		require.NoError(t, err)
		assert.Equal(t, []tracestore.Operation{{Name: "blue-op"}}, operations)
	}
}

func TestFailedInitialLoadIsNotCached(t *testing.T) {
	r, backend, _ := newTestReader(t)
	backend.On("GetServices", mock.Anything).Return(nil, errors.New("storage unavailable")).Once()
	backend.On("GetServices", mock.Anything).Return([]string{"a"}, nil).Once()

	_, err := r.GetServices(t.Context())
	require.EqualError(t, err, "storage unavailable")

	services, err := r.GetServices(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"a"}, services)
}

func TestFailedRefreshServesCachedValues(t *testing.T) {
	r, backend, clock := newTestReader(t)
	core, logs := observer.New(zapcore.WarnLevel)
	r.logger = zap.New(core)
	backend.On("GetServices", mock.Anything).Return([]string{"a"}, nil).Once()
	backend.On("GetServices", mock.Anything).Return(nil, errors.New("storage unavailable")).Once()
	backend.On("GetServices", mock.Anything).Return([]string{"a", "b"}, nil).Once()

	_, err := r.GetServices(t.Context())
	require.NoError(t, err)

	clock.Advance(testTTL)
	services, err := r.GetServices(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"a"}, services)
	require.Equal(t, 1, logs.Len())
	assert.Equal(t, "storage unavailable", logs.All()[0].ContextMap()["error"])

	services, err = r.GetServices(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, services)
}

func TestEmptyResultsAreNotCached(t *testing.T) {
	r, backend, clock := newTestReader(t)
	query := tracestore.OperationQueryParams{ServiceName: "svc"}
	backend.On("GetOperations", mock.Anything, query).Return(nil, nil).Once()
	backend.On("GetOperations", mock.Anything, query).Return([]tracestore.Operation{{Name: "op"}}, nil).Once()
	backend.On("GetOperations", mock.Anything, query).Return([]tracestore.Operation{}, nil).Once()
	backend.On("GetOperations", mock.Anything, query).Return([]tracestore.Operation{{Name: "new-op"}}, nil).Once()

	operations, err := r.GetOperations(t.Context(), query)
	require.NoError(t, err)
	assert.Empty(t, operations)

	operations, err = r.GetOperations(t.Context(), query)
	require.NoError(t, err)
	assert.Equal(t, []tracestore.Operation{{Name: "op"}}, operations)

	clock.Advance(testTTL)
	operations, err = r.GetOperations(t.Context(), query)
	require.NoError(t, err)
	assert.Empty(t, operations)

	operations, err = r.GetOperations(t.Context(), query)
	require.NoError(t, err)
	assert.Equal(t, []tracestore.Operation{{Name: "new-op"}}, operations)
}

func TestCallersCannotModifyCachedValues(t *testing.T) {
	r, backend, _ := newTestReader(t)
	backend.On("GetServices", mock.Anything).Return([]string{"a", "b"}, nil).Once()

	loaded, err := r.GetServices(t.Context())
	require.NoError(t, err)
	loaded[0] = "changed-after-load"

	cached, err := r.GetServices(t.Context())
	require.NoError(t, err)
	cached[1] = "changed-after-hit"

	services, err := r.GetServices(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, services)
}

func TestCallersGetCachedValuesWhileAnotherRefreshes(t *testing.T) {
	r, backend, clock := newTestReader(t)
	backend.On("GetServices", mock.Anything).Return([]string{"a"}, nil).Once()
	_, err := r.GetServices(t.Context())
	require.NoError(t, err)

	refreshStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	backend.On("GetServices", mock.Anything).
		Run(func(mock.Arguments) {
			close(refreshStarted)
			<-releaseRefresh
		}).
		Return([]string{"a", "b"}, nil).
		Once()

	clock.Advance(testTTL)
	refreshed := make(chan []string)
	go func() {
		services, err := r.GetServices(context.Background())
		assert.NoError(t, err)
		refreshed <- services
	}()
	<-refreshStarted

	services, err := r.GetServices(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"a"}, services)

	close(releaseRefresh)
	assert.Equal(t, []string{"a", "b"}, <-refreshed)

	services, err = r.GetServices(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, services)
}

func TestOtherMethodsGoToTheWrappedReader(t *testing.T) {
	r, backend, _ := newTestReader(t)
	backend.On("SearchCapabilities", mock.Anything).
		Return(tracestore.SearchCapabilities{WithoutServiceName: true}, nil).Once()

	capabilities, err := r.SearchCapabilities(t.Context())
	require.NoError(t, err)
	assert.True(t, capabilities.WithoutServiceName)
}
